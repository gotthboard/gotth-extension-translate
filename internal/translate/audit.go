package translate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const (
	auditQueueCapacity  = 128
	auditShutdownBudget = 100 * time.Millisecond
)

type Event struct {
	Name              string
	Code              string
	CorrelationDigest string
	SegmentCount      int
	SourceBytes       int
	OutputBytes       int
	DurationBucket    string
}

type Recorder interface{ Record(Event) }

type AuditStats struct {
	Dropped uint64
	Failed  uint64
}

// LogRecorder owns one bounded queue and one writer goroutine. Record never
// waits for the output sink. Close has a hard deadline, so a blocked stderr
// writer cannot stall an RPC or process shutdown.
type LogRecorder struct {
	writer  io.Writer
	queue   chan []byte
	stop    chan struct{}
	done    chan struct{}
	closing atomic.Bool
	dropped atomic.Uint64
	failed  atomic.Uint64
	once    sync.Once
}

func NewLogRecorder(writer io.Writer) *LogRecorder {
	r := &LogRecorder{writer: writer, queue: make(chan []byte, auditQueueCapacity), stop: make(chan struct{}), done: make(chan struct{})}
	go r.run()
	return r
}

func (r *LogRecorder) run() {
	defer close(r.done)
	for {
		select {
		case line := <-r.queue:
			if r.writer != nil {
				if _, err := r.writer.Write(line); err != nil {
					r.failed.Add(1)
				}
			}
		case <-r.stop:
			for {
				select {
				case line := <-r.queue:
					if r.writer != nil {
						if _, err := r.writer.Write(line); err != nil {
							r.failed.Add(1)
						}
					}
				default:
					return
				}
			}
		}
	}
}

func (r *LogRecorder) Record(event Event) {
	if r == nil || r.writer == nil || r.closing.Load() {
		return
	}
	line := map[string]any{
		"event":              event.Name,
		"code":               event.Code,
		"correlation_digest": event.CorrelationDigest,
		"segment_count":      event.SegmentCount,
		"source_bytes":       event.SourceBytes,
		"output_bytes":       event.OutputBytes,
		"duration_bucket":    event.DurationBucket,
	}
	encoded, err := json.Marshal(line)
	if err != nil {
		r.failed.Add(1)
		return
	}
	encoded = append(encoded, byte(10))
	select {
	case r.queue <- encoded:
	default:
		r.dropped.Add(1)
	}
}

func (r *LogRecorder) Stats() AuditStats {
	if r == nil {
		return AuditStats{}
	}
	return AuditStats{Dropped: r.dropped.Load(), Failed: r.failed.Load()}
}

func (r *LogRecorder) Close() error {
	if r == nil {
		return nil
	}
	r.closing.Store(true)
	r.once.Do(func() { close(r.stop) })
	ctx, cancel := context.WithTimeout(context.Background(), auditShutdownBudget)
	defer cancel()
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return errors.New("audit shutdown deadline exceeded")
	}
}

func digestCorrelation(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func latencyBucket(elapsed time.Duration) string {
	switch {
	case elapsed < 10*time.Millisecond:
		return "lt_10ms"
	case elapsed < 50*time.Millisecond:
		return "10_49ms"
	case elapsed < 250*time.Millisecond:
		return "50_249ms"
	case elapsed < time.Second:
		return "250_999ms"
	case elapsed < 5*time.Second:
		return "1_4s"
	default:
		return "ge_5s"
	}
}

type discardRecorder struct{}

func (discardRecorder) Record(Event) {}
