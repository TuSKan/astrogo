package xmatch

import "testing"

// TestUnionFind_UnionByRankBothBranches exercises both non-tie branches of
// union's rank comparison — Match's own tests only ever union small,
// evenly-shaped groups, which happens to always hit the tie ("default")
// branch. Building a rank-1 root first, then unioning it against a fresh
// rank-0 element in both x/y orders, is the minimal case that forces each
// branch.
func TestUnionFind_UnionByRankBothBranches(t *testing.T) {
	t.Run("rank[rx] > rank[ry]", func(t *testing.T) {
		uf := newUnionFind(3)
		uf.union(0, 1) // tie -> default branch, rank[find(0)] becomes 1
		uf.union(0, 2) // rank[find(0)]=1 > rank[find(2)]=0 -> ">" branch

		if uf.find(0) != uf.find(2) {
			t.Error("expected 0 and 2 to be unioned")
		}
	})

	t.Run("rank[rx] < rank[ry]", func(t *testing.T) {
		uf := newUnionFind(3)
		uf.union(1, 2) // tie -> default branch, rank[find(1)] becomes 1
		uf.union(0, 1) // rank[find(0)]=0 < rank[find(1)]=1 -> "<" branch

		if uf.find(0) != uf.find(1) {
			t.Error("expected 0 and 1 to be unioned")
		}
	})
}
