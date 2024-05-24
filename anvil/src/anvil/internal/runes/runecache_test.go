package runes

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type anAppend struct {
	textToAppend    string
	expectedOffsets []offsetCacheEntry
}

func TestBuildCache(t *testing.T) {
	tests := []struct {
		name       string
		initialDoc string
		interval   int
		appends    []anAppend
	}{
		{
			name:       "basic",
			initialDoc: "123412341234123",
			interval:   4,
			appends: []anAppend{
				{
					textToAppend: "",
					expectedOffsets: []offsetCacheEntry{
						{0, 0},
						{4, 4},
						{8, 8},
						{12, 12},
					},
				},
				{
					textToAppend: "4",
					expectedOffsets: []offsetCacheEntry{
						{0, 0},
						{4, 4},
						{8, 8},
						{12, 12},
						{16, 16},
					},
				},
				{
					textToAppend: "1",
					expectedOffsets: []offsetCacheEntry{
						{0, 0},
						{4, 4},
						{8, 8},
						{12, 12},
						{16, 16},
					},
				},
				{
					textToAppend: "23412341",
					expectedOffsets: []offsetCacheEntry{
						{0, 0},
						{4, 4},
						{8, 8},
						{12, 12},
						{16, 16},
						{20, 20},
						{24, 24},
					},
				},
			},
		},
		{
			name:       "empty",
			initialDoc: "",
			interval:   4,
			appends: []anAppend{
				{
					textToAppend: "",
					expectedOffsets: []offsetCacheEntry{
						{0, 0},
					},
				},
				{
					textToAppend: "123412341234123",
					expectedOffsets: []offsetCacheEntry{
						{0, 0},
						{4, 4},
						{8, 8},
						{12, 12},
					},
				},
				{
					textToAppend: "4",
					expectedOffsets: []offsetCacheEntry{
						{0, 0},
						{4, 4},
						{8, 8},
						{12, 12},
						{16, 16},
					},
				},
				{
					textToAppend: "1",
					expectedOffsets: []offsetCacheEntry{
						{0, 0},
						{4, 4},
						{8, 8},
						{12, 12},
						{16, 16},
					},
				},
				{
					textToAppend: "23412341",
					expectedOffsets: []offsetCacheEntry{
						{0, 0},
						{4, 4},
						{8, 8},
						{12, 12},
						{16, 16},
						{20, 20},
						{24, 24},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := NewOffsetCache(tc.interval)
			doc := []byte(tc.initialDoc)
			cache.Update(doc)

			for i, ap := range tc.appends {

				doc = append(doc, []byte(ap.textToAppend)...)
				cache.Update(doc)

				assert.Equal(t, ap.expectedOffsets, cache.vals, fmt.Sprintf("on append %d", i))
			}
		})
	}
}

type query struct {
	runeOffset         int
	expectedByteOffset int
}

func TestQueryCache(t *testing.T) {
	tests := []struct {
		name       string
		initialDoc string
		interval   int
		queries    []query
	}{
		{
			name:       "emptydoc",
			initialDoc: "",
			interval:   4,
			queries: []query{
				{
					runeOffset:         0,
					expectedByteOffset: 0,
				},
				{
					runeOffset:         3,
					expectedByteOffset: 0,
				},
			},
		},
		{
			name:       "basic",
			initialDoc: "123412341234123",
			interval:   4,
			queries: []query{
				{
					runeOffset:         0,
					expectedByteOffset: 0,
				},
				{
					runeOffset:         3,
					expectedByteOffset: 3,
				},
				{
					runeOffset:         5,
					expectedByteOffset: 5,
				},
				{
					runeOffset:         9,
					expectedByteOffset: 9,
				},
				{
					runeOffset:         14,
					expectedByteOffset: 14,
				},
			},
		},
		{
			name:       "two-byte runes",
			initialDoc: "§§§§§§§§",
			interval:   4,
			queries: []query{
				{
					runeOffset:         0,
					expectedByteOffset: 0,
				},
				{
					runeOffset:         3,
					expectedByteOffset: 6,
				},
				{
					runeOffset:         5,
					expectedByteOffset: 10,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := NewOffsetCache(tc.interval)
			doc := []byte(tc.initialDoc)
			cache.Update(doc)

			for i, q := range tc.queries {
				boff, err, _ := cache.Get(doc, q.runeOffset)
				assert.Equal(t, nil, err)
				assert.Equal(t, q.expectedByteOffset, boff, fmt.Sprintf("on query %d", i))
			}
		})
	}
}
