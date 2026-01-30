package telemetry

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

type Emitter struct {
	config *EmitterConfig
	writer *bufio.Writer
	file   *os.File
	mu     sync.Mutex

	totalWritten atomic.Int64
	totalBytes   atomic.Int64
	writeErrors  atomic.Int64
}

func NewEmitter(config *EmitterConfig) (*Emitter, error) {
	if config == nil {
		config = DefaultEmitterConfig()
	}

	e := &Emitter{
		config: config,
	}

	if config.OutputPath != "" {
		f, err := os.OpenFile(config.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		e.file = f
		e.writer = bufio.NewWriterSize(f, config.BufferSize)
	}

	return e, nil
}

func NewEmitterWithWriter(w io.Writer, config *EmitterConfig) *Emitter {
	if config == nil {
		config = DefaultEmitterConfig()
	}

	return &Emitter{
		config: config,
		writer: bufio.NewWriterSize(w, config.BufferSize),
	}
}

func (e *Emitter) EmitOpLog(log *OpLog) error {
	data, err := log.MarshalJSONL()
	if err != nil {
		e.writeErrors.Add(1)
		return err
	}

	return e.writeLine(data)
}

func (e *Emitter) EmitWorkerHealth(health *WorkerHealth) error {
	data, err := health.MarshalJSONL()
	if err != nil {
		e.writeErrors.Add(1)
		return err
	}

	return e.writeLine(data)
}

func (e *Emitter) EmitRecord(record *TelemetryRecord) error {
	var data []byte
	var err error

	switch record.Type {
	case "op_log":
		if record.OpLog != nil {
			data, err = record.OpLog.MarshalJSONL()
		}
	case "worker_health":
		if record.WorkerHealth != nil {
			data, err = record.WorkerHealth.MarshalJSONL()
		}
	default:
		data, err = json.Marshal(record)
	}

	if err != nil {
		e.writeErrors.Add(1)
		return err
	}

	return e.writeLine(data)
}

func (e *Emitter) EmitBatch(batch *TelemetryBatch) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.writer == nil {
		return nil
	}

	for _, log := range batch.Records {
		data, err := log.MarshalJSONL()
		if err != nil {
			e.writeErrors.Add(1)
			continue
		}

		if _, err := e.writer.Write(data); err != nil {
			e.writeErrors.Add(1)
			return err
		}
		if err := e.writer.WriteByte('\n'); err != nil {
			e.writeErrors.Add(1)
			return err
		}

		e.totalWritten.Add(1)
		e.totalBytes.Add(int64(len(data) + 1))
	}

	if batch.WorkerHealth != nil {
		data, err := batch.WorkerHealth.MarshalJSONL()
		if err == nil {
			if _, err := e.writer.Write(data); err != nil {
				e.writeErrors.Add(1)
				return err
			}
			if err := e.writer.WriteByte('\n'); err != nil {
				e.writeErrors.Add(1)
				return err
			}
			e.totalWritten.Add(1)
			e.totalBytes.Add(int64(len(data) + 1))
		}
	}

	if e.config.SyncOnWrite {
		return e.flushLocked()
	}

	return nil
}

func (e *Emitter) writeLine(data []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.writer == nil {
		return nil
	}

	if _, err := e.writer.Write(data); err != nil {
		e.writeErrors.Add(1)
		return err
	}
	if err := e.writer.WriteByte('\n'); err != nil {
		e.writeErrors.Add(1)
		return err
	}

	e.totalWritten.Add(1)
	e.totalBytes.Add(int64(len(data) + 1))

	if e.config.SyncOnWrite {
		return e.flushLocked()
	}

	return nil
}

func (e *Emitter) flushLocked() error {
	if e.writer == nil {
		return nil
	}

	if err := e.writer.Flush(); err != nil {
		return err
	}

	if e.file != nil {
		return e.file.Sync()
	}

	return nil
}

func (e *Emitter) Flush() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.flushLocked()
}

func (e *Emitter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.writer != nil {
		if err := e.writer.Flush(); err != nil {
			return err
		}
	}

	if e.file != nil {
		return e.file.Close()
	}

	return nil
}

type EmitterStats struct {
	TotalWritten int64
	TotalBytes   int64
	WriteErrors  int64
}

func (e *Emitter) Stats() EmitterStats {
	return EmitterStats{
		TotalWritten: e.totalWritten.Load(),
		TotalBytes:   e.totalBytes.Load(),
		WriteErrors:  e.writeErrors.Load(),
	}
}
