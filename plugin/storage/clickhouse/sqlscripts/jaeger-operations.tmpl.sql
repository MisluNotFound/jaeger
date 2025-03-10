CREATE MATERIALIZED VIEW IF NOT EXISTS {{.OperationsTable}}
{{if .Replication}}ON CLUSTER '{{.Cluster}}'{{end}}
TO {{.OperationsSummaryTable}}
AS SELECT
    {{if .Multitenant -}}
    tenant,
    {{- end -}}
    toDate(timestamp) AS date,
    service,
    operation,
    count() AS count,
    if(
        has(tags.key, 'span.kind'),
        tags.value[indexOf(tags.key, 'span.kind')],
        ''
    ) AS spankind
FROM {{.Database}}.{{.SpansIndexTable}}
GROUP BY
    {{if .Multitenant -}}
    tenant,
    {{- end -}}
    date,
    service,
    operation,
    tags.key,
    tags.value
