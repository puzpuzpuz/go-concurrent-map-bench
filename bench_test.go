package bench

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphadose/haxmap"
	"github.com/cornelk/hashmap"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/puzpuzpuz/xsync/v4"
)

const benchmarkKeyPrefix = "what_a_looooooooooooooooooooooong_key_prefix_"

var benchmarkSizes = []int{100, 1_000, 100_000, 1_000_000}

// cornelk/hashmap becomes extremely slow with writes at large map sizes,
// so we cap its benchmarks at 1,000 entries.
var benchmarkSizesCornelk = []int{100, 1_000}

var benchmarkCases = []struct {
	name           string
	readPercentage int
}{
	{"reads=100%", 100},
	{"reads=99%", 99},
	{"reads=90%", 90},
	{"reads=75%", 75},
}

var (
	maxSize    = benchmarkSizes[len(benchmarkSizes)-1]
	stringKeys []string
	intKeys    []int
)

func init() {
	stringKeys = make([]string, maxSize)
	intKeys = make([]int, maxSize)
	for i := range maxSize {
		stringKeys[i] = benchmarkKeyPrefix + strconv.Itoa(i)
		intKeys[i] = i
	}
}

func benchmarkStringKeys(
	b *testing.B,
	numKeys int,
	loadFn func(k string) (int, bool),
	storeFn func(k string, v int),
	deleteFn func(k string),
	readPercentage int,
) {
	b.ResetTimer()
	start := time.Now()
	b.RunParallel(func(pb *testing.PB) {
		// Convert percent to permille to support 99% case.
		storeThreshold := 10 * readPercentage
		deleteThreshold := 10*readPercentage + ((1000 - 10*readPercentage) / 2)
		numKeysU32 := uint32(numKeys)
		var sink int
		for pb.Next() {
			op := int(rand.Uint32() % 1000)
			i := int(rand.Uint32() % numKeysU32)
			if op >= deleteThreshold {
				deleteFn(stringKeys[i])
			} else if op >= storeThreshold {
				storeFn(stringKeys[i], i)
			} else {
				v, _ := loadFn(stringKeys[i])
				sink += v
			}
		}
		_ = sink
	})
	opsPerSec := float64(b.N) / time.Since(start).Seconds()
	b.ReportMetric(opsPerSec, "ops/s")
}

func benchmarkIntKeys(
	b *testing.B,
	numKeys int,
	loadFn func(k int) (int, bool),
	storeFn func(k int, v int),
	deleteFn func(k int),
	readPercentage int,
) {
	b.ResetTimer()
	start := time.Now()
	b.RunParallel(func(pb *testing.PB) {
		storeThreshold := 10 * readPercentage
		deleteThreshold := 10*readPercentage + ((1000 - 10*readPercentage) / 2)
		numKeysU32 := uint32(numKeys)
		var sink int
		for pb.Next() {
			op := int(rand.Uint32() % 1000)
			i := int(rand.Uint32() % numKeysU32)
			if op >= deleteThreshold {
				deleteFn(intKeys[i])
			} else if op >= storeThreshold {
				storeFn(intKeys[i], i)
			} else {
				v, _ := loadFn(intKeys[i])
				sink += v
			}
		}
		_ = sink
	})
	opsPerSec := float64(b.N) / time.Since(start).Seconds()
	b.ReportMetric(opsPerSec, "ops/s")
}

func benchmarkRangeStringKeys(
	b *testing.B,
	numKeys int,
	storeFn func(k string, v int),
	rangeFn func(func(string, int) bool),
) {
	var stopped atomic.Bool
	var wg sync.WaitGroup
	numKeysU32 := uint32(numKeys)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stopped.Load() {
			i := int(rand.Uint32() % numKeysU32)
			storeFn(stringKeys[i], i)
		}
	}()
	b.ResetTimer()
	start := time.Now()
	b.RunParallel(func(pb *testing.PB) {
		var sink int
		for pb.Next() {
			rangeFn(func(_ string, v int) bool {
				sink += v
				return true
			})
		}
		_ = sink
	})
	b.StopTimer()
	stopped.Store(true)
	wg.Wait()
	opsPerSec := float64(b.N) / time.Since(start).Seconds()
	b.ReportMetric(opsPerSec, "ops/s")
}

func benchmarkRangeIntKeys(
	b *testing.B,
	numKeys int,
	storeFn func(k int, v int),
	rangeFn func(func(int, int) bool),
) {
	var stopped atomic.Bool
	var wg sync.WaitGroup
	numKeysU32 := uint32(numKeys)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stopped.Load() {
			i := int(rand.Uint32() % numKeysU32)
			storeFn(intKeys[i], i)
		}
	}()
	b.ResetTimer()
	start := time.Now()
	b.RunParallel(func(pb *testing.PB) {
		var sink int
		for pb.Next() {
			rangeFn(func(_ int, v int) bool {
				sink += v
				return true
			})
		}
		_ = sink
	})
	b.StopTimer()
	stopped.Store(true)
	wg.Wait()
	opsPerSec := float64(b.N) / time.Since(start).Seconds()
	b.ReportMetric(opsPerSec, "ops/s")
}

// sync.Map

func BenchmarkSyncMap_WarmUp_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				b.Run(bc.name, func(b *testing.B) {
					var m sync.Map
					for i := range size {
						m.Store(stringKeys[i], i)
					}
					benchmarkStringKeys(b, size,
						func(k string) (int, bool) {
							v, ok := m.Load(k)
							if ok {
								return v.(int), true
							}
							return 0, false
						},
						func(k string, v int) { m.Store(k, v) },
						func(k string) { m.Delete(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkSyncMap_NoWarmUp_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				if bc.readPercentage == 100 {
					continue
				}
				b.Run(bc.name, func(b *testing.B) {
					var m sync.Map
					benchmarkStringKeys(b, size,
						func(k string) (int, bool) {
							v, ok := m.Load(k)
							if ok {
								return v.(int), true
							}
							return 0, false
						},
						func(k string, v int) { m.Store(k, v) },
						func(k string) { m.Delete(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkSyncMap_WarmUp_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				b.Run(bc.name, func(b *testing.B) {
					var m sync.Map
					for i := range size {
						m.Store(intKeys[i], i)
					}
					benchmarkIntKeys(b, size,
						func(k int) (int, bool) {
							v, ok := m.Load(k)
							if ok {
								return v.(int), true
							}
							return 0, false
						},
						func(k int, v int) { m.Store(k, v) },
						func(k int) { m.Delete(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkSyncMap_NoWarmUp_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				if bc.readPercentage == 100 {
					continue
				}
				b.Run(bc.name, func(b *testing.B) {
					var m sync.Map
					benchmarkIntKeys(b, size,
						func(k int) (int, bool) {
							v, ok := m.Load(k)
							if ok {
								return v.(int), true
							}
							return 0, false
						},
						func(k int, v int) { m.Store(k, v) },
						func(k int) { m.Delete(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

// xsync.MapOf

func BenchmarkXsyncMapOf_WarmUp_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				b.Run(bc.name, func(b *testing.B) {
					m := xsync.NewMap[string, int](xsync.WithPresize(size))
					for i := range size {
						m.Store(stringKeys[i], i)
					}
					benchmarkStringKeys(b, size,
						func(k string) (int, bool) { return m.Load(k) },
						func(k string, v int) { m.Store(k, v) },
						func(k string) { m.Delete(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkXsyncMapOf_NoWarmUp_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				if bc.readPercentage == 100 {
					continue
				}
				b.Run(bc.name, func(b *testing.B) {
					m := xsync.NewMap[string, int]()
					benchmarkStringKeys(b, size,
						func(k string) (int, bool) { return m.Load(k) },
						func(k string, v int) { m.Store(k, v) },
						func(k string) { m.Delete(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkXsyncMapOf_WarmUp_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				b.Run(bc.name, func(b *testing.B) {
					m := xsync.NewMap[int, int](xsync.WithPresize(size))
					for i := range size {
						m.Store(intKeys[i], i)
					}
					benchmarkIntKeys(b, size,
						func(k int) (int, bool) { return m.Load(k) },
						func(k int, v int) { m.Store(k, v) },
						func(k int) { m.Delete(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkXsyncMapOf_NoWarmUp_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				if bc.readPercentage == 100 {
					continue
				}
				b.Run(bc.name, func(b *testing.B) {
					m := xsync.NewMap[int, int]()
					benchmarkIntKeys(b, size,
						func(k int) (int, bool) { return m.Load(k) },
						func(k int, v int) { m.Store(k, v) },
						func(k int) { m.Delete(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

// cornelk/hashmap

func BenchmarkCornelkHashmap_WarmUp_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizesCornelk {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				b.Run(bc.name, func(b *testing.B) {
					m := hashmap.NewSized[string, int](uintptr(size))
					for i := range size {
						m.Set(stringKeys[i], i)
					}
					benchmarkStringKeys(b, size,
						func(k string) (int, bool) { return m.Get(k) },
						func(k string, v int) { m.Set(k, v) },
						func(k string) { m.Del(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkCornelkHashmap_NoWarmUp_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizesCornelk {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				if bc.readPercentage == 100 {
					continue
				}
				b.Run(bc.name, func(b *testing.B) {
					m := hashmap.New[string, int]()
					benchmarkStringKeys(b, size,
						func(k string) (int, bool) { return m.Get(k) },
						func(k string, v int) { m.Set(k, v) },
						func(k string) { m.Del(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkCornelkHashmap_WarmUp_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizesCornelk {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				b.Run(bc.name, func(b *testing.B) {
					m := hashmap.NewSized[int, int](uintptr(size))
					for i := range size {
						m.Set(intKeys[i], i)
					}
					benchmarkIntKeys(b, size,
						func(k int) (int, bool) { return m.Get(k) },
						func(k int, v int) { m.Set(k, v) },
						func(k int) { m.Del(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkCornelkHashmap_NoWarmUp_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizesCornelk {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				if bc.readPercentage == 100 {
					continue
				}
				b.Run(bc.name, func(b *testing.B) {
					m := hashmap.New[int, int]()
					benchmarkIntKeys(b, size,
						func(k int) (int, bool) { return m.Get(k) },
						func(k int, v int) { m.Set(k, v) },
						func(k int) { m.Del(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

// alphadose/haxmap

func BenchmarkHaxmap_WarmUp_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				b.Run(bc.name, func(b *testing.B) {
					m := haxmap.New[string, int](uintptr(size))
					for i := range size {
						m.Set(stringKeys[i], i)
					}
					benchmarkStringKeys(b, size,
						func(k string) (int, bool) { return m.Get(k) },
						func(k string, v int) { m.Set(k, v) },
						func(k string) { m.Del(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkHaxmap_NoWarmUp_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				if bc.readPercentage == 100 {
					continue
				}
				b.Run(bc.name, func(b *testing.B) {
					m := haxmap.New[string, int]()
					benchmarkStringKeys(b, size,
						func(k string) (int, bool) { return m.Get(k) },
						func(k string, v int) { m.Set(k, v) },
						func(k string) { m.Del(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkHaxmap_WarmUp_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				b.Run(bc.name, func(b *testing.B) {
					m := haxmap.New[int, int](uintptr(size))
					for i := range size {
						m.Set(intKeys[i], i)
					}
					benchmarkIntKeys(b, size,
						func(k int) (int, bool) { return m.Get(k) },
						func(k int, v int) { m.Set(k, v) },
						func(k int) { m.Del(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkHaxmap_NoWarmUp_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				if bc.readPercentage == 100 {
					continue
				}
				b.Run(bc.name, func(b *testing.B) {
					m := haxmap.New[int, int]()
					benchmarkIntKeys(b, size,
						func(k int) (int, bool) { return m.Get(k) },
						func(k int, v int) { m.Set(k, v) },
						func(k int) { m.Del(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

// orcaman/concurrent-map

func BenchmarkOrcamanCmap_WarmUp_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				b.Run(bc.name, func(b *testing.B) {
					m := cmap.New[int]()
					for i := range size {
						m.Set(stringKeys[i], i)
					}
					benchmarkStringKeys(b, size,
						func(k string) (int, bool) { return m.Get(k) },
						func(k string, v int) { m.Set(k, v) },
						func(k string) { m.Remove(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkOrcamanCmap_NoWarmUp_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				if bc.readPercentage == 100 {
					continue
				}
				b.Run(bc.name, func(b *testing.B) {
					m := cmap.New[int]()
					benchmarkStringKeys(b, size,
						func(k string) (int, bool) { return m.Get(k) },
						func(k string, v int) { m.Set(k, v) },
						func(k string) { m.Remove(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkOrcamanCmap_WarmUp_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				b.Run(bc.name, func(b *testing.B) {
					m := cmap.NewWithCustomShardingFunction[int, int](func(key int) uint32 {
						return uint32(key)
					})
					for i := range size {
						m.Set(intKeys[i], i)
					}
					benchmarkIntKeys(b, size,
						func(k int) (int, bool) { return m.Get(k) },
						func(k int, v int) { m.Set(k, v) },
						func(k int) { m.Remove(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

func BenchmarkOrcamanCmap_NoWarmUp_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, bc := range benchmarkCases {
				if bc.readPercentage == 100 {
					continue
				}
				b.Run(bc.name, func(b *testing.B) {
					m := cmap.NewWithCustomShardingFunction[int, int](func(key int) uint32 {
						return uint32(key)
					})
					benchmarkIntKeys(b, size,
						func(k int) (int, bool) { return m.Get(k) },
						func(k int, v int) { m.Set(k, v) },
						func(k int) { m.Remove(k) },
						bc.readPercentage,
					)
				})
			}
		})
	}
}

// Range benchmarks: multiple goroutines iterate while one goroutine writes.

// sync.Map

func BenchmarkSyncMap_Range_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			var m sync.Map
			for i := range size {
				m.Store(stringKeys[i], i)
			}
			benchmarkRangeStringKeys(b, size,
				func(k string, v int) { m.Store(k, v) },
				func(f func(string, int) bool) {
					m.Range(func(key, value any) bool {
						return f(key.(string), value.(int))
					})
				},
			)
		})
	}
}

func BenchmarkSyncMap_Range_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			var m sync.Map
			for i := range size {
				m.Store(intKeys[i], i)
			}
			benchmarkRangeIntKeys(b, size,
				func(k int, v int) { m.Store(k, v) },
				func(f func(int, int) bool) {
					m.Range(func(key, value any) bool {
						return f(key.(int), value.(int))
					})
				},
			)
		})
	}
}

// xsync.MapOf

func BenchmarkXsyncMapOf_Range_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			m := xsync.NewMap[string, int](xsync.WithPresize(size))
			for i := range size {
				m.Store(stringKeys[i], i)
			}
			benchmarkRangeStringKeys(b, size,
				func(k string, v int) { m.Store(k, v) },
				func(f func(string, int) bool) { m.RangeRelaxed(f) },
			)
		})
	}
}

func BenchmarkXsyncMapOf_Range_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			m := xsync.NewMap[int, int](xsync.WithPresize(size))
			for i := range size {
				m.Store(intKeys[i], i)
			}
			benchmarkRangeIntKeys(b, size,
				func(k int, v int) { m.Store(k, v) },
				func(f func(int, int) bool) { m.RangeRelaxed(f) },
			)
		})
	}
}

// cornelk/hashmap

func BenchmarkCornelkHashmap_Range_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizesCornelk {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			m := hashmap.NewSized[string, int](uintptr(size))
			for i := range size {
				m.Set(stringKeys[i], i)
			}
			benchmarkRangeStringKeys(b, size,
				func(k string, v int) { m.Set(k, v) },
				func(f func(string, int) bool) {
					m.Range(func(k string, v int) bool {
						return f(k, v)
					})
				},
			)
		})
	}
}

func BenchmarkCornelkHashmap_Range_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizesCornelk {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			m := hashmap.NewSized[int, int](uintptr(size))
			for i := range size {
				m.Set(intKeys[i], i)
			}
			benchmarkRangeIntKeys(b, size,
				func(k int, v int) { m.Set(k, v) },
				func(f func(int, int) bool) {
					m.Range(func(k int, v int) bool {
						return f(k, v)
					})
				},
			)
		})
	}
}

// alphadose/haxmap

func BenchmarkHaxmap_Range_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			m := haxmap.New[string, int](uintptr(size))
			for i := range size {
				m.Set(stringKeys[i], i)
			}
			benchmarkRangeStringKeys(b, size,
				func(k string, v int) { m.Set(k, v) },
				func(f func(string, int) bool) { m.ForEach(f) },
			)
		})
	}
}

func BenchmarkHaxmap_Range_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			m := haxmap.New[int, int](uintptr(size))
			for i := range size {
				m.Set(intKeys[i], i)
			}
			benchmarkRangeIntKeys(b, size,
				func(k int, v int) { m.Set(k, v) },
				func(f func(int, int) bool) { m.ForEach(f) },
			)
		})
	}
}

// orcaman/concurrent-map

func BenchmarkOrcamanCmap_Range_StringKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			m := cmap.New[int]()
			for i := range size {
				m.Set(stringKeys[i], i)
			}
			benchmarkRangeStringKeys(b, size,
				func(k string, v int) { m.Set(k, v) },
				func(f func(string, int) bool) {
					for t := range m.IterBuffered() {
						if !f(t.Key, t.Val) {
							break
						}
					}
				},
			)
		})
	}
}

func BenchmarkOrcamanCmap_Range_IntKeys(b *testing.B) {
	for _, size := range benchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			m := cmap.NewWithCustomShardingFunction[int, int](func(key int) uint32 {
				return uint32(key)
			})
			for i := range size {
				m.Set(intKeys[i], i)
			}
			benchmarkRangeIntKeys(b, size,
				func(k int, v int) { m.Set(k, v) },
				func(f func(int, int) bool) {
					for t := range m.IterBuffered() {
						if !f(t.Key, t.Val) {
							break
						}
					}
				},
			)
		})
	}
}

