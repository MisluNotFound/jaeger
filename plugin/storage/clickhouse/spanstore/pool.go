package spanstore

import (
	"math"
	"sync"

	"github.com/jaegertracing/jaeger/model"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

var (
	numDiscardedSpans = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "jaeger_clickhouse_discarded_spans",
		Help: "Count of spans that have been discarded due to pending writes exceeding max_span_count",
	})
	numPendingSpans = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "jaeger_clickhouse_pending_spans",
		Help: "Number of spans that are currently pending, counts against max_span_count",
	})
)

// WriteWorkerPool is a worker pool for writing batches of spans.
// Given a new batch, WriteWorkerPool creates a new WriteWorker.
// If the number of currently processed spans if more than maxSpanCount, then the oldest worker is removed.
type WriteWorkerPool struct {
	params *WorkerParams

	finish  chan bool
	done    sync.WaitGroup
	batches chan []*model.Span

	maxSpanCount int
	mutex        sync.Mutex
	workers      workerHeap
	workerDone   chan *WriteWorker
}

var registerPoolMetrics sync.Once

func NewWorkerPool(params *WorkerParams, maxSpanCount int) WriteWorkerPool {
	registerPoolMetrics.Do(func() {
		prometheus.MustRegister(numDiscardedSpans, numPendingSpans)
	})

	return WriteWorkerPool{
		params:  params,
		finish:  make(chan bool),
		done:    sync.WaitGroup{},
		batches: make(chan []*model.Span),

		mutex:      sync.Mutex{},
		workers:    newWorkerHeap(100),
		workerDone: make(chan *WriteWorker),

		maxSpanCount: maxSpanCount,
	}
}

func (pool *WriteWorkerPool) Work() {
	finish := false
	nextWorkerID := int32(1)
	pendingSpanCount := 0
	for {
		// Initialize to zero, or update value from previous loop
		numPendingSpans.Set(float64(pendingSpanCount))

		pool.done.Add(1)
		select {
		case batch := <-pool.batches:
			batchSize := len(batch)
			if pool.checkLimit(pendingSpanCount, batchSize) {
				// Limit disabled or batch fits within limit, write the batch.
				worker := WriteWorker{
					workerID: nextWorkerID,

					params: pool.params,
					batch:  batch,

					finish:     make(chan bool),
					workerDone: pool.workerDone,
					done:       sync.WaitGroup{},
				}
				if nextWorkerID == math.MaxInt32 {
					nextWorkerID = 1
				} else {
					nextWorkerID++
				}
				pool.workers.AddWorker(&worker)
				pendingSpanCount += batchSize
				go worker.Work()
			} else {
				// Limit exceeded, complain
				numDiscardedSpans.Add(float64(batchSize))
				pool.params.logger.Error("Discarding batch of spans due to exceeding pending span count", zap.Any("batch_size", batchSize), zap.Any("pending_span_count", pendingSpanCount), zap.Any("max_span_count", pool.maxSpanCount))
			}
		case worker := <-pool.workerDone:
			// The worker has finished, subtract its work from the count and clean it from the heap.
			pendingSpanCount -= len(worker.batch)
			if err := pool.workers.RemoveWorker(worker); err != nil {
				pool.params.logger.Error("could not remove worker", zap.Any("worker", worker), zap.Any("error", err))
			}
		case <-pool.finish:
			pool.workers.CloseWorkers()
			finish = true
		}
		pool.done.Done()

		if finish {
			break
		}
	}
}

func (pool *WriteWorkerPool) WriteBatch(batch []*model.Span) {
	pool.batches <- batch
}

func (pool *WriteWorkerPool) Close() {
	pool.finish <- true
	pool.done.Wait()
}

// checkLimit returns whether batchSize fits within the maxSpanCount
func (pool *WriteWorkerPool) checkLimit(pendingSpanCount int, batchSize int) bool {
	if pool.maxSpanCount <= 0 {
		return true
	}

	// Check limit, add batchSize if within limit
	return pendingSpanCount+batchSize <= pool.maxSpanCount
}
