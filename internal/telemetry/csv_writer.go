package telemetry

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// LogRecord represents a single empirical observation logged to CSV.
type LogRecord struct {
	Timestamp      time.Time
	Mode           string
	PayloadSize    int
	Concurrency    int
	LatencyUs      float64
	Checksum       uint64
	HeapAllocBytes uint64
	SysMemBytes    uint64
	NumGC          uint32
}

// CSVWriter manages asynchronous, buffered flushing of telemetry records to disk.
type CSVWriter struct {
	file       *os.File
	writer     *csv.Writer
	recordChan chan LogRecord
	doneChan   chan struct{}
	wg         sync.WaitGroup
	flushMu    sync.Mutex
}

// NewCSVWriter opens or creates the CSV destination file and spins up a background writer.
func NewCSVWriter(filePath string, bufferCap int) (*CSVWriter, error) {
	if bufferCap <= 0 {
		bufferCap = 50000
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create telemetry directory: %w", err)
	}

	fileExists := false
	if _, err := os.Stat(filePath); err == nil {
		fileExists = true
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open telemetry file: %w", err)
	}

	w := csv.NewWriter(file)

	// Write CSV Header if newly created
	if !fileExists {
		header := []string{
			"timestamp",
			"mode",
			"payload_size_bytes",
			"concurrency",
			"latency_us",
			"checksum",
			"heap_alloc_bytes",
			"sys_mem_bytes",
			"num_gc",
		}
		if err := w.Write(header); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to write CSV header: %w", err)
		}
		w.Flush()
	}

	cw := &CSVWriter{
		file:       file,
		writer:     w,
		recordChan: make(chan LogRecord, bufferCap),
		doneChan:   make(chan struct{}),
	}

	cw.wg.Add(1)
	go cw.worker()

	return cw, nil
}

// Log enqueues a record to be written. Drops if queue is saturated to prevent blocking hot paths.
func (cw *CSVWriter) Log(rec LogRecord) {
	select {
	case cw.recordChan <- rec:
	default:
		// Queue full under extreme pressure: drop to preserve latency fidelity
	}
}

func (cw *CSVWriter) worker() {
	defer cw.wg.Done()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case rec, ok := <-cw.recordChan:
			if !ok {
				cw.flushRecords()
				return
			}
			cw.writeRecord(rec)
		case <-ticker.C:
			cw.flushMu.Lock()
			cw.writer.Flush()
			cw.flushMu.Unlock()
		case <-cw.doneChan:
			// Drain remaining records
			for {
				select {
				case rec := <-cw.recordChan:
					cw.writeRecord(rec)
				default:
					cw.flushRecords()
					return
				}
			}
		}
	}
}

func (cw *CSVWriter) writeRecord(rec LogRecord) {
	cw.flushMu.Lock()
	defer cw.flushMu.Unlock()

	row := []string{
		rec.Timestamp.UTC().Format(time.RFC3339Nano),
		rec.Mode,
		strconv.Itoa(rec.PayloadSize),
		strconv.Itoa(rec.Concurrency),
		strconv.FormatFloat(rec.LatencyUs, 'f', 4, 64),
		strconv.FormatUint(rec.Checksum, 10),
		strconv.FormatUint(rec.HeapAllocBytes, 10),
		strconv.FormatUint(rec.SysMemBytes, 10),
		strconv.FormatUint(uint64(rec.NumGC), 10),
	}
	_ = cw.writer.Write(row)
}

func (cw *CSVWriter) flushRecords() {
	cw.flushMu.Lock()
	defer cw.flushMu.Unlock()
	cw.writer.Flush()
	_ = cw.file.Sync()
}

// Close gracefully drains the channel and syncs the file to disk.
func (cw *CSVWriter) Close() error {
	close(cw.doneChan)
	cw.wg.Wait()
	cw.flushRecords()
	return cw.file.Close()
}
