package spanstore

import (
	"container/heap"
	"fmt"
	"time"
)

var (
	_                 heap.Interface = workerHeap{}
	errWorkerNotFound                = fmt.Errorf("worker not found in heap")
)

type heapItem struct {
	pushTime time.Time
	worker   *WriteWorker
}

// workerHeap is a heap for WriteWorkers where worker's push time is the key.
type workerHeap struct {
	elems   *[]*heapItem
	indexes map[*WriteWorker]int
}

func newWorkerHeap(cap int) workerHeap {
	elems := make([]*heapItem, 0, cap)
	return workerHeap{
		elems:   &elems,
		indexes: make(map[*WriteWorker]int),
	}
}

func (workerHeap workerHeap) AddWorker(worker *WriteWorker) {
	heap.Push(workerHeap, heapItem{
		worker:   worker,
		pushTime: time.Now(),
	})
}

func (workerHeap *workerHeap) RemoveWorker(worker *WriteWorker) error {
	idx, ok := workerHeap.indexes[worker]
	if !ok {
		return errWorkerNotFound
	}
	heap.Remove(workerHeap, idx)
	return nil
}

func (workerHeap *workerHeap) CloseWorkers() {
	for _, item := range *workerHeap.elems {
		item.worker.Close()
	}
}

func (workerHeap workerHeap) Len() int {
	return len(*workerHeap.elems)
}

func (workerHeap workerHeap) Less(i, j int) bool {
	return (*workerHeap.elems)[i].pushTime.Before((*workerHeap.elems)[j].pushTime)
}

func (workerHeap workerHeap) Swap(i, j int) {
	(*workerHeap.elems)[i], (*workerHeap.elems)[j] = (*workerHeap.elems)[j], (*workerHeap.elems)[i]
	workerHeap.indexes[(*workerHeap.elems)[i].worker] = i
	workerHeap.indexes[(*workerHeap.elems)[j].worker] = j
}

func (workerHeap workerHeap) Push(x interface{}) {
	switch t := x.(type) {
	case heapItem:
		*workerHeap.elems = append(*workerHeap.elems, &t)
		workerHeap.indexes[t.worker] = len(*workerHeap.elems) - 1
	default:
		panic("Unknown type")
	}
}

func (workerHeap workerHeap) Pop() interface{} {
	lastInd := len(*workerHeap.elems) - 1
	last := (*workerHeap.elems)[lastInd]
	delete(workerHeap.indexes, last.worker)
	*workerHeap.elems = (*workerHeap.elems)[:lastInd]
	return last.worker
}
