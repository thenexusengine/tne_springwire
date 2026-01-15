// Package middleware provides HTTP middleware for PBS
package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// GzipConfig holds gzip compression configuration
type GzipConfig struct {
	Enabled       bool
	MinLength     int      // Minimum response size to compress (bytes)
	Level         int      // Compression level (1-9, default 6)
	ContentTypes  []string // Content types to compress
	ExcludedPaths []string // Paths to exclude from compression
}

// DefaultGzipConfig returns default gzip configuration
func DefaultGzipConfig() *GzipConfig {
	return &GzipConfig{
		Enabled:   true,
		MinLength: 256, // Don't compress responses smaller than 256 bytes
		Level:     6,   // Balanced compression level
		ContentTypes: []string{
			"application/json",
			"text/plain",
			"text/html",
		},
		ExcludedPaths: []string{
			"/metrics", // Prometheus metrics are already efficient
			"/health",  // Health checks should be fast
			"/status",  // Status checks should be fast
		},
	}
}

// Gzip provides response compression middleware
type Gzip struct {
	config     *GzipConfig
	writerPool sync.Pool
}

// NewGzip creates a new Gzip middleware
func NewGzip(config *GzipConfig) *Gzip {
	if config == nil {
		config = DefaultGzipConfig()
	}

	level := config.Level
	if level < 1 || level > 9 {
		level = 6
	}

	return &Gzip{
		config: config,
		writerPool: sync.Pool{
			New: func() interface{} {
				w, err := gzip.NewWriterLevel(io.Discard, level)
				if err != nil {
					return nil
				}
				return w
			},
		},
	}
}

// gzipResponseWriter wraps http.ResponseWriter for compression
// It buffers the response to decide whether to compress based on size
type gzipResponseWriter struct {
	http.ResponseWriter
	gzipWriter  *gzip.Writer
	config      *GzipConfig
	writerPool  *sync.Pool
	buffer      bytes.Buffer
	wroteHeader bool
	headerCode  int
	shouldGzip  bool
}

// Header returns the header map
func (grw *gzipResponseWriter) Header() http.Header {
	return grw.ResponseWriter.Header()
}

// WriteHeader captures status code but defers actual header write
func (grw *gzipResponseWriter) WriteHeader(code int) {
	if grw.wroteHeader {
		return
	}
	grw.headerCode = code
}

// Write buffers data and decides on compression
func (grw *gzipResponseWriter) Write(b []byte) (int, error) {
	// Buffer the data
	n, err := grw.buffer.Write(b)
	return n, err
}

// shouldCompress checks if content type should be compressed
func (grw *gzipResponseWriter) shouldCompress(contentType string) bool {
	if contentType == "" {
		return false
	}

	// Extract base content type (without charset, etc.)
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	for _, ct := range grw.config.ContentTypes {
		if strings.EqualFold(ct, contentType) {
			return true
		}
	}
	return false
}

// Flush writes buffered content to the underlying writer, compressing if appropriate
func (grw *gzipResponseWriter) Flush() error {
	if grw.wroteHeader {
		return nil // Already flushed
	}
	grw.wroteHeader = true

	data := grw.buffer.Bytes()
	contentType := grw.Header().Get("Content-Type")

	// Decide whether to compress
	grw.shouldGzip = len(data) >= grw.config.MinLength && grw.shouldCompress(contentType)

	if grw.shouldGzip && grw.gzipWriter != nil {
		// Set compression headers
		grw.Header().Set("Content-Encoding", "gzip")
		grw.Header().Del("Content-Length") // Length changes after compression
		grw.Header().Add("Vary", "Accept-Encoding")

		// Write status code
		if grw.headerCode == 0 {
			grw.headerCode = http.StatusOK
		}
		grw.ResponseWriter.WriteHeader(grw.headerCode)

		// Compress and write data
		grw.gzipWriter.Reset(grw.ResponseWriter)
		_, err := grw.gzipWriter.Write(data)
		if err != nil {
			return err
		}
		return grw.gzipWriter.Close()
	}

	// Write without compression
	if grw.headerCode == 0 {
		grw.headerCode = http.StatusOK
	}
	grw.ResponseWriter.WriteHeader(grw.headerCode)
	_, err := grw.ResponseWriter.Write(data)
	return err
}

// Close returns the gzip writer to the pool
func (grw *gzipResponseWriter) Close() error {
	// Flush any remaining data
	if !grw.wroteHeader {
		if err := grw.Flush(); err != nil {
			return err
		}
	}

	// Return gzip writer to pool
	if grw.gzipWriter != nil {
		grw.gzipWriter.Reset(io.Discard)
		grw.writerPool.Put(grw.gzipWriter)
	}
	return nil
}

// Middleware returns the gzip compression middleware handler
func (g *Gzip) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip if disabled
		if !g.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Skip excluded paths
		for _, path := range g.config.ExcludedPaths {
			if strings.HasPrefix(r.URL.Path, path) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Get a gzip writer from pool
		poolWriter := g.writerPool.Get()
		gzipWriter, ok := poolWriter.(*gzip.Writer)
		if !ok || gzipWriter == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		grw := &gzipResponseWriter{
			ResponseWriter: w,
			gzipWriter:     gzipWriter,
			config:         g.config,
			writerPool:     &g.writerPool,
		}
		defer grw.Close()

		next.ServeHTTP(grw, r)
	})
}
