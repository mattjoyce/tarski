package datalog_test

import "github.com/mattjoyce/shearline/datalog"

// The oracles the fuzz tests check the engine against: plain Go over a small boolean
// matrix, sharing nothing with the evaluator, so they can disagree with it.

// oracleNodes is the graph size the fuzz tests use. Small enough that a matrix is the
// obvious representation, large enough for cycles, self-edges and long paths.
const oracleNodes = 8

// matrix is a directed graph: m[i][j] is an edge from node i to node j.
type matrix [oracleNodes][oracleNodes]bool

// graphFrom reads data two bytes at a time as edges between nodes mod oracleNodes, and
// returns the matrix and the same edges as edge/2 facts, duplicates included.
func graphFrom(data []byte) (matrix, []datalog.Fact) {
	var m matrix
	var facts []datalog.Fact
	for i := 0; i+1 < len(data); i += 2 {
		a, b := int(data[i]%oracleNodes), int(data[i+1]%oracleNodes)
		m[a][b] = true
		facts = append(facts, datalog.Fact{Relation: "edge", Tuple: ints(a, b)})
	}
	return m, facts
}

// nodeFacts is node/1 for every node, whether or not an edge touches it.
func nodeFacts() []datalog.Fact {
	facts := make([]datalog.Fact, oracleNodes)
	for i := range facts {
		facts[i] = datalog.Fact{Relation: "node", Tuple: ints(i)}
	}
	return facts
}

// closure is the transitive closure by Floyd-Warshall: reach[i][j] holds when some
// non-empty path leads from i to j, which is what the two reach/2 rules derive.
func (m matrix) closure() matrix {
	for k := 0; k < oracleNodes; k++ {
		for i := 0; i < oracleNodes; i++ {
			for j := 0; j < oracleNodes; j++ {
				m[i][j] = m[i][j] || m[i][k] && m[k][j]
			}
		}
	}
	return m
}

func ints(vs ...int) datalog.Tuple {
	t := make(datalog.Tuple, len(vs))
	for i, v := range vs {
		t[i] = datalog.Int(int64(v))
	}
	return t
}
