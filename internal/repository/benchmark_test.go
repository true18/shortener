package repository

import (
	"fmt"
	"testing"
)

var benchmarkShortID string

func BenchmarkMemorySave(b *testing.B) {
	urls := benchmarkRepositoryURLs(512)
	store := NewMemory()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, err := store.Save(fmt.Sprintf("%s/%d", urls[i%len(urls)], i), "benchmark-user")
		if err != nil {
			b.Fatal(err)
		}
		benchmarkShortID = id
	}
}

func BenchmarkMemoryFind(b *testing.B) {
	store := NewMemory()
	ids := make([]string, 1024)
	for i := range ids {
		id, err := store.Save(fmt.Sprintf("https://example.com/item/%d?source=bench", i), "benchmark-user")
		if err != nil {
			b.Fatal(err)
		}
		ids[i] = id
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		url, err := store.Find(ids[i%len(ids)])
		if err != nil {
			b.Fatal(err)
		}
		if url == "" {
			b.Fatal("empty URL")
		}
	}
}

func benchmarkRepositoryURLs(count int) []string {
	urls := make([]string, count)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://example.com/catalog/%d?category=books&campaign=benchmark", i)
	}

	return urls
}
