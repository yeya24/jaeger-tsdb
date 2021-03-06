package main

import (
	"context"
	"errors"
	"flag"
	"math"
	"time"

	"github.com/conprof/tsdb"
	"github.com/conprof/tsdb/labels"
	"github.com/conprof/tsdb/wal"
	"github.com/go-kit/kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// ErrServiceNameNotSet occurs when attempting to query with an empty service name
	ErrServiceNameNotSet = errors.New("service name must be set")

	// ErrStartTimeMinGreaterThanMax occurs when start time min is above start time max
	ErrStartTimeMinGreaterThanMax = errors.New("min start time is above max")

	// ErrDurationMinGreaterThanMax occurs when duration min is above duration max
	ErrDurationMinGreaterThanMax = errors.New("min duration is above max")

	// ErrMalformedRequestObject occurs when a request object is nil
	ErrMalformedRequestObject = errors.New("malformed request object")

	// ErrStartAndEndTimeNotSet occurs when start time and end time are not set
	ErrStartAndEndTimeNotSet = errors.New("start and end time must be set")

	// ErrUnableToFindTraceIDAggregation occurs when an aggregation query for TraceIDs fail.
	ErrUnableToFindTraceIDAggregation = errors.New("could not find aggregation of traceIDs")

	// ErrNotSupported during development, don't support every option - yet
	ErrNotSupported = errors.New("this query parameter is not supported yet")

	storagePath = flag.String("storage.tsdb.path", "/Users/yeya24/prom/data1", "Directory to read storage from.")
)

const (
	serviceLabel   = "__svc__"
	operationLabel = "__op__"
	traceIDLabel   = "__traceid__"
)

type store struct {
	conf   *conf
	tsdb   *tsdb.DB
}

type conf struct {
	retention   time.Duration
	storagePath string
}

func newStore() (*store, error) {
	c := &conf{
		retention:  2 * time.Hour,
		storagePath: "/data",
	}

	tdb, err := tsdb.Open(
		c.storagePath,
		log.NewJSONLogger(logger.StandardWriter(&hclog.StandardLoggerOptions{ForceLevel: hclog.Warn})),
		prometheus.DefaultRegisterer,
		&tsdb.Options{
			WALSegmentSize:    wal.DefaultSegmentSize,
			RetentionDuration: uint64(c.retention),
			BlockRanges:       append([]int64{int64(10 * time.Minute), int64(1 * time.Hour)}, tsdb.ExponentialBlockRanges(int64(2*time.Hour)/1e6, 3, 5)...),
			NoLockfile:        true,
		},
	)
	if err != nil {
		logger.Warn("failed to open tsdb", "err", err)
		return nil, err
	}
	return &store{tsdb: tdb, conf: c}, nil
}

func (s *store) Close() error {
	return s.tsdb.Close()
}

func (s *store) DependencyReader() dependencystore.Reader {
	return s
}

func (s *store) SpanReader() spanstore.Reader {
	return s
}

func (s *store) SpanWriter() spanstore.Writer {
	return s
}

func (s *store) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return nil, nil
}

func (s *store) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	q, err := s.tsdb.Querier(0, math.MaxInt64)
	if err != nil {
		logger.Warn("failed to create querier", "err", err)
		return nil, err
	}
	ss, err := q.Select(labels.NewEqualMatcher(traceIDLabel, traceID.String()))
	if err != nil {
		logger.Warn("failed to select", "err", err)
		return nil, err
	}

	var spans []*model.Span
	for ss.Next() {
		series := ss.At()
		it := series.Iterator()
		for it.Next() {
			_, v := it.At()
			span, err := decodeValue(v)
			if err != nil {
				logger.Warn("failed to unmarshal to span", "err", err)
				return nil, err
			}
			spans = append(spans, span)
		}
		if err := it.Err(); err != nil {
			logger.Warn("failed to iterate series set", "err", err)
			return nil, err
		}
	}

	return &model.Trace{Spans: spans}, nil
}

func (s *store) GetServices(ctx context.Context) ([]string, error) {
	q, err := s.tsdb.Querier(0, math.MaxInt64)
	if err != nil {
		return nil, err
	}
	return q.LabelValues(serviceLabel)
}

func (s *store) GetOperations(ctx context.Context, service string) ([]string, error) {
		q, err := s.tsdb.Querier(0, math.MaxInt64)
		if err != nil {
			logger.Warn("failed to create querier", "err", err)
			return nil, err
		}

		var lbs []labels.Matcher
		if service != "" {
			lbs = append(lbs, labels.NewEqualMatcher(serviceLabel, service))
		} else {
			// if service is "", get all the operations.
			lbs = append(lbs, labels.Not(labels.NewEqualMatcher(serviceLabel, "")))
		}

		logger.Warn("find ops for service", "service", service)

		var res []string
		ss, err := q.Select(lbs...)
		if err != nil {
			logger.Warn("failed to select", "err", err)
			return nil, err
		}

		opMap := make(map[string]struct{})
		for ss.Next() {
			series := ss.At()
			ls := series.Labels()
			operation := ls.Get(operationLabel)
			if _, ok := opMap[operation]; !ok {
				opMap[operation] = struct{}{}
				res = append(res, operation)
			}
		}

		return res, nil
}


func (s *store) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	ss, err := s.selectTSDBWithQuery(query)
	if err != nil {
		logger.Warn("failed to select", "err", err)
		return nil, err
	}

	traceMap := make(map[string]*model.Trace)
	for ss.Next() {
		it := ss.At().Iterator()
		for it.Next() {
			_, v := it.At()
			span, err := decodeValue(v)
			if err != nil {
				logger.Warn("failed to unmarshal to span", "err", err)
				return nil, err
			}

			id := span.TraceID.String()
			if _, ok := traceMap[id]; !ok {
				traceMap[id] = &model.Trace{Spans: []*model.Span{span}}
			} else {
				traceMap[id].Spans = append(traceMap[id].Spans, span)
			}
		}
		if err := it.Err(); err != nil {
			logger.Warn("failed to iterate series set", "err", err)
			return nil, err
		}
	}

	var res []*model.Trace
	for _, v := range traceMap {
		res = append(res, v)
	}

	return res, nil
}

func (s *store) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	ss, err := s.selectTSDBWithQuery(query)
	if err != nil {
		logger.Warn("failed to select", "err", err)
		return nil, err
	}

	var res []model.TraceID
	for ss.Next() {
		series := ss.At()
		ls := series.Labels()
		tid, err := model.TraceIDFromString(ls.Get(traceIDLabel))
		if err != nil {
			// log err here
			logger.Warn("failed to generate traceid", "err", err)
			return nil, err
		}
		res = append(res, tid)
	}

	return res, nil
}

func (s *store) WriteSpan(span *model.Span) error {
	start := time.Now()
	data, err := proto.Marshal(span)
	if err != nil {
		return err
	}

	app := s.tsdb.Appender()
	ls := make(labels.Labels, 0)

	ls = append(ls, labels.Label{serviceLabel, span.GetProcess().ServiceName})
	ls = append(ls, labels.Label{operationLabel, span.GetOperationName()})
	ls = append(ls, labels.Label{traceIDLabel, span.TraceID.String()})

	for _, kv := range span.Tags {
		ls = append(ls, labels.Label{kv.Key, kv.AsString()})
	}

	for _, kv := range span.Process.Tags {
		ls = append(ls, labels.Label{kv.Key, kv.AsString()})
	}

	for _, lg := range span.Logs {
		for _, kv := range lg.Fields {
			ls = append(ls, labels.Label{kv.Key, kv.AsString()})
		}
	}

	if _, err = app.Add(ls, int64(model.TimeAsEpochMicroseconds(span.StartTime)), data); err != nil {
		logger.Warn("error write to tsdb", "err", err)
		return err
	}

	if err = app.Commit(); err != nil {
		logger.Warn("error commit to tsdb", "err", err)
		return err
	}
	logger.Warn("write to tsdb", "duration", time.Since(start).String(),
		"service", span.GetProcess().ServiceName, "operation", span.GetOperationName(),
		"trace", span.TraceID.String(), "time", int64(model.TimeAsEpochMicroseconds(span.StartTime)))

	return nil
}

func validateQuery(p *spanstore.TraceQueryParameters) error {
	if p == nil {
		return ErrMalformedRequestObject
	}
	if p.ServiceName == "" {
		return ErrServiceNameNotSet
	}

	if p.StartTimeMin.IsZero() || p.StartTimeMax.IsZero() {
		return ErrStartAndEndTimeNotSet
	}

	if p.StartTimeMax.Before(p.StartTimeMin) {
		return ErrStartTimeMinGreaterThanMax
	}
	if p.DurationMin != 0 && p.DurationMax != 0 && p.DurationMin > p.DurationMax {
		return ErrDurationMinGreaterThanMax
	}
	return nil
}

func (s *store) selectTSDBWithQuery(query *spanstore.TraceQueryParameters) (tsdb.SeriesSet, error) {
	if err := validateQuery(query); err != nil {
		logger.Warn("query is invalid", "err", err)
		return nil, err
	}
	q, err := s.tsdb.Querier(int64(model.TimeAsEpochMicroseconds(query.StartTimeMin)), int64(model.TimeAsEpochMicroseconds(query.StartTimeMax)))
	if err != nil {
		logger.Warn("failed to create querier", "err", err)
		return nil, err
	}
	defer q.Close()

	var lbs []labels.Matcher
	if query.OperationName != "" {
		lbs = append(lbs, labels.NewEqualMatcher(operationLabel, query.OperationName))
	}

	if query.ServiceName != "" {
		lbs = append(lbs, labels.NewEqualMatcher(serviceLabel, query.ServiceName))
	}

	for k, v := range query.Tags {
		lbs = append(lbs, labels.NewEqualMatcher(k, v))
	}

	ss, err := q.Select(lbs...)
	if err != nil {
		return nil, err
	}
	return ss, nil
}

func decodeValue(val []byte) (*model.Span, error) {
	sp := model.Span{}
	if err := proto.Unmarshal(val, &sp); err != nil {
		return nil, err
	}
	return &sp, nil
}