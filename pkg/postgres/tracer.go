package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"ride-hail-system/pkg/metrics"
)

type dbStartKey struct{}

type MetricsTracer struct{}

func (t *MetricsTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, dbStartKey{}, struct {
		start time.Time
		sql   string
	}{time.Now(), data.SQL})
}

func (t *MetricsTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	v, ok := ctx.Value(dbStartKey{}).(struct {
		start time.Time
		sql   string
	})
	if !ok {
		return
	}
	metrics.RecordDBQuery(sqlOperation(v.sql), time.Since(v.start))
}

func sqlOperation(sql string) string {
	sql = strings.TrimSpace(sql)
	idx := strings.IndexByte(sql, ' ')
	if idx < 0 {
		idx = len(sql)
	}
	return strings.ToLower(sql[:idx])
}
