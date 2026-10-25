package tarski_test

import (
	"strconv"
	"testing"

	"github.com/mattjoyce/tarski"
)

// BenchmarkNewBase_Attr measures building a base the size of Anam's attr/3 on the
// live record (hundreds of thousands of tuples), one Fact per tuple through NewBase.
// It answers Anam's ask for a bulk constructor: build one only if this is too slow.
func BenchmarkNewBase_Attr(b *testing.B) {
	for _, n := range []int{100_000, 500_000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			schema := tarski.Schema{Arities: map[string]int{"attr": 3}}
			facts := make([]tarski.Fact, n)
			for i := range facts {
				v, _ := tarski.Number(strconv.Itoa(i % 1000))
				facts[i] = tarski.Fact{Relation: "attr", Tuple: tarski.Tuple{
					tarski.String("facet-" + strconv.Itoa(i/8)),
					tarski.String("/field/" + strconv.Itoa(i%8)),
					v,
				}}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := tarski.NewBase(schema, facts...); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
