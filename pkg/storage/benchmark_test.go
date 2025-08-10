package storage

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Benchmark configurations
var benchmarkSizes = []struct {
	name string
	size int64
}{
	{"1KB", 1024},
	{"100KB", 100 * 1024},
	{"1MB", 1024 * 1024},
	{"10MB", 10 * 1024 * 1024},
	{"100MB", 100 * 1024 * 1024},
}

func BenchmarkFileSystemStorage_PutObject(b *testing.B) {
	for _, bs := range benchmarkSizes {
		b.Run(bs.name, func(b *testing.B) {
			tempDir := b.TempDir()
			storage, err := NewFileSystemStorage(tempDir)
			if err != nil {
				b.Fatal(err)
			}

			// Create bucket once
			_, _ = storage.CreateBucket("bench-bucket")

			// Generate random data once
			data := make([]byte, bs.size)
			_, _ = rand.Read(data)

			b.ResetTimer()
			b.SetBytes(bs.size)

			for i := 0; i < b.N; i++ {
				reader := bytes.NewReader(data)
				_, err := storage.PutObject("bench-bucket", fmt.Sprintf("object-%d", i), reader, bs.size, "application/octet-stream")
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFileSystemStorage_GetObject(b *testing.B) {
	for _, bs := range benchmarkSizes {
		b.Run(bs.name, func(b *testing.B) {
			tempDir := b.TempDir()
			storage, err := NewFileSystemStorage(tempDir)
			if err != nil {
				b.Fatal(err)
			}

			// Setup: create bucket and object
			_, _ = storage.CreateBucket("bench-bucket")
			data := make([]byte, bs.size)
			_, _ = rand.Read(data)
			reader := bytes.NewReader(data)
			_, _ = storage.PutObject("bench-bucket", "test-object", reader, bs.size, "application/octet-stream")

			b.ResetTimer()
			b.SetBytes(bs.size)

			for i := 0; i < b.N; i++ {
				reader, _, err := storage.GetObject("bench-bucket", "test-object")
				if err != nil {
					b.Fatal(err)
				}
				// Consume the data
				_, _ = io.Copy(io.Discard, reader)
				reader.Close()
			}
		})
	}
}

func BenchmarkMemoryStorage_PutObject(b *testing.B) {
	for _, bs := range benchmarkSizes {
		b.Run(bs.name, func(b *testing.B) {
			storage := NewMemoryStorage()

			// Create bucket once
			_, _ = storage.CreateBucket("bench-bucket")

			// Generate random data once
			data := make([]byte, bs.size)
			_, _ = rand.Read(data)

			b.ResetTimer()
			b.SetBytes(bs.size)

			for i := 0; i < b.N; i++ {
				reader := bytes.NewReader(data)
				_, err := storage.PutObject("bench-bucket", fmt.Sprintf("object-%d", i), reader, bs.size, "application/octet-stream")
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMemoryStorage_GetObject(b *testing.B) {
	for _, bs := range benchmarkSizes {
		b.Run(bs.name, func(b *testing.B) {
			storage := NewMemoryStorage()

			// Setup: create bucket and object
			_, _ = storage.CreateBucket("bench-bucket")
			data := make([]byte, bs.size)
			_, _ = rand.Read(data)
			reader := bytes.NewReader(data)
			_, _ = storage.PutObject("bench-bucket", "test-object", reader, bs.size, "application/octet-stream")

			b.ResetTimer()
			b.SetBytes(bs.size)

			for i := 0; i < b.N; i++ {
				reader, _, err := storage.GetObject("bench-bucket", "test-object")
				if err != nil {
					b.Fatal(err)
				}
				// Consume the data
				_, _ = io.Copy(io.Discard, reader)
				reader.Close()
			}
		})
	}
}

// Benchmark concurrent operations
func BenchmarkFileSystemStorage_ConcurrentPutObject(b *testing.B) {
	tempDir := b.TempDir()
	storage, err := NewFileSystemStorage(tempDir)
	if err != nil {
		b.Fatal(err)
	}

	_, _ = storage.CreateBucket("bench-bucket")

	// 1MB objects
	size := int64(1024 * 1024)
	data := make([]byte, size)
	_, _ = rand.Read(data)

	b.SetBytes(size)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			reader := bytes.NewReader(data)
			_, err := storage.PutObject("bench-bucket", fmt.Sprintf("object-%d", i), reader, size, "application/octet-stream")
			if err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

// Benchmark multipart uploads
func BenchmarkFileSystemStorage_MultipartUpload(b *testing.B) {
	for _, bs := range benchmarkSizes {
		if bs.size < 1024*1024 {
			continue // Skip small sizes for multipart
		}
		b.Run(bs.name, func(b *testing.B) {
			tempDir := b.TempDir()
			storage, err := NewFileSystemStorage(tempDir)
			if err != nil {
				b.Fatal(err)
			}

			_, _ = storage.CreateBucket("bench-bucket")

			// Prepare data
			data := make([]byte, bs.size)
			_, _ = rand.Read(data)

			partSize := int64(5 * 1024 * 1024) // 5MB parts
			numParts := (bs.size + partSize - 1) / partSize

			b.ResetTimer()
			b.SetBytes(bs.size)

			for i := 0; i < b.N; i++ {
				uploadId, err := storage.InitiateMultipartUpload("bench-bucket", fmt.Sprintf("multipart-%d", i))
				if err != nil {
					b.Fatal(err)
				}

				var parts []CompletedPart
				for p := 0; p < int(numParts); p++ {
					start := int64(p) * partSize
					end := start + partSize
					if end > bs.size {
						end = bs.size
					}

					reader := bytes.NewReader(data[start:end])
					etag, err := storage.UploadPart("bench-bucket", fmt.Sprintf("multipart-%d", i), uploadId, p+1, reader, end-start)
					if err != nil {
						b.Fatal(err)
					}

					parts = append(parts, CompletedPart{
						PartNumber: p + 1,
						ETag:       etag,
					})
				}

				_, err = storage.CompleteMultipartUpload("bench-bucket", fmt.Sprintf("multipart-%d", i), uploadId, parts)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Helper function to create large test file
func createTempFile(size int64) (string, error) {
	tempFile, err := os.CreateTemp("", "bench-*.dat")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	// Write random data in chunks to avoid memory issues
	chunkSize := int64(1024 * 1024) // 1MB chunks
	chunk := make([]byte, chunkSize)

	for written := int64(0); written < size; {
		toWrite := chunkSize
		if written+toWrite > size {
			toWrite = size - written
		}

		_, _ = rand.Read(chunk[:toWrite])
		if _, err := tempFile.Write(chunk[:toWrite]); err != nil {
			os.Remove(tempFile.Name())
			return "", err
		}
		written += toWrite
	}

	return tempFile.Name(), nil
}

// Benchmark memory usage for large files
func BenchmarkFileSystemStorage_LargeFileMemoryUsage(b *testing.B) {
	tempDir := b.TempDir()
	storage, err := NewFileSystemStorage(tempDir)
	if err != nil {
		b.Fatal(err)
	}

	_, _ = storage.CreateBucket("bench-bucket")

	// Create a 500MB test file
	size := int64(500 * 1024 * 1024)
	tempFile, err := createTempFile(size)
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tempFile)

	b.ResetTimer()
	b.SetBytes(size)

	for i := 0; i < b.N; i++ {
		file, err := os.Open(tempFile)
		if err != nil {
			b.Fatal(err)
		}

		_, err = storage.PutObject("bench-bucket", fmt.Sprintf("large-%d", i), file, size, "application/octet-stream")
		file.Close()
		if err != nil {
			b.Fatal(err)
		}

		// Clean up to avoid disk space issues
		_ = storage.DeleteObject("bench-bucket", fmt.Sprintf("large-%d", i))
	}
}

// Benchmark list operations with many objects
func BenchmarkFileSystemStorage_ListObjects(b *testing.B) {
	objectCounts := []int{100, 1000, 10000}

	for _, count := range objectCounts {
		b.Run(fmt.Sprintf("%d_objects", count), func(b *testing.B) {
			tempDir := b.TempDir()
			storage, err := NewFileSystemStorage(tempDir)
			if err != nil {
				b.Fatal(err)
			}

			// Setup: create bucket and many objects
			_, _ = storage.CreateBucket("bench-bucket")
			data := []byte("test data")
			for i := 0; i < count; i++ {
				reader := bytes.NewReader(data)
				_, _ = storage.PutObject("bench-bucket", fmt.Sprintf("object-%06d", i), reader, int64(len(data)), "text/plain")
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _, _, err := storage.ListObjects("bench-bucket", "", "", 1000, "")
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Benchmark with different directory depths
func BenchmarkFileSystemStorage_DeepDirectoryStructure(b *testing.B) {
	depths := []int{1, 5, 10}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			tempDir := b.TempDir()
			storage, err := NewFileSystemStorage(tempDir)
			if err != nil {
				b.Fatal(err)
			}

			_, _ = storage.CreateBucket("bench-bucket")

			// Create path with specified depth
			var path string
			for d := 0; d < depth; d++ {
				path = filepath.Join(path, fmt.Sprintf("dir%d", d))
			}
			path = filepath.Join(path, "file.txt")

			data := []byte("test data")

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				reader := bytes.NewReader(data)
				key := filepath.Join(path, fmt.Sprintf("file-%d.txt", i))
				_, err := storage.PutObject("bench-bucket", key, reader, int64(len(data)), "text/plain")
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
