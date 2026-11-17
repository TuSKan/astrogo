package plan

import (
	"math"
	"slices"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
)

// star is a placed star at (ra, dec) degrees, of apparent magnitude mag, a
// member of the objects named in memberOf.
func star(id string, ra, dec, mag float64, memberOf ...string) placedObject {
	return placed(id, resolve.KindStar, ra, dec, mag, memberOf...)
}

func placed(id string, kind resolve.Kind, ra, dec, mag float64, memberOf ...string) placedObject {
	pos := coord.NewICRS(angle.Deg(ra), angle.Deg(dec))

	return placedObject{
		pos: pos,
		vo: VisibleObject{
			Target:      resolve.Target{ID: id, Name: id, Kind: kind, Coord: pos, HasCoord: true, MemberOf: memberOf},
			ApparentMag: mag,
		},
	}
}

// alfCen is alf Cen's system record and its two components as SIMBAD gives
// them: positions and V from its basic and allfluxes tables, and the
// components' membership from h_link, read through its CfA mirror on
// 2026-11-17. The system's V -0.1 is A's and B's light together.
func alfCen() []placedObject {
	return []placedObject{
		star("* alf Cen", 219.9020833333333, -60.83397222222223, -0.1),
		star("* alf Cen A", 219.90205833170774, -60.83399268831004, 0.01, "* alf Cen"),
		star("* alf Cen B", 219.89609628987276, -60.83752756558407, 1.33, "* alf Cen"),
	}
}

// acrux is alf01 and alf02 Cru as SIMBAD gives them, read the same way: two
// stars a few arcseconds apart that h_link makes members of nothing.
func acrux() []placedObject {
	return []placedObject{
		star("* alf01 Cru", 186.6495666666667, -63.09909166666667, 1.28),
		star("* alf02 Cru", 186.65184166666666, -63.09952222222222, 1.58),
	}
}

func memberIDs(vo VisibleObject) []string {
	ids := make([]string, len(vo.Members))
	for i, m := range vo.Members {
		ids[i] = m.Target.ID
	}

	return ids
}

func rowByID(t *testing.T, rows []VisibleObject, id string) VisibleObject {
	t.Helper()

	for _, r := range rows {
		if r.Target.ID == id {
			return r
		}
	}

	t.Fatalf("no row for %q among %d rows", id, len(rows))

	return VisibleObject{}
}

// TestAStarSystemIsOneRowAtItsOwnMagnitude: to the naked eye alf Cen A, B and
// their system's record are one point of light. They are one row, the
// system's, at the system's V, which already holds A's and B's light, so
// nothing is added twice; all three are its members, brightest first (#1182).
func TestAStarSystemIsOneRowAtItsOwnMagnitude(t *testing.T) {
	t.Parallel()

	rows := observingGroups(alfCen(), eyeResolution)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want alf Cen as one", len(rows))
	}

	row := rows[0]
	if row.Target.ID != "* alf Cen" || row.ApparentMag != -0.1 {
		t.Errorf("row is %q at %v, want the system's record at its own V -0.1", row.Target.ID, row.ApparentMag)
	}

	if want := []string{"* alf Cen", "* alf Cen A", "* alf Cen B"}; !slices.Equal(memberIDs(row), want) {
		t.Errorf("members = %q, want %q", memberIDs(row), want)
	}

	for _, m := range row.Members {
		if m.Members != nil {
			t.Errorf("member %q has members of its own", m.Target.ID)
		}
	}
}

// TestDistinctStarsCombineTheirLight: alf01 and alf02 Cru are two stars that
// no hierarchy relates, one point of light to the eye. Their row is the
// brighter, at their combined magnitude: 10^(-0.4*1.28) + 10^(-0.4*1.58) is
// 0.540955, which is magnitude 0.667096.
func TestDistinctStarsCombineTheirLight(t *testing.T) {
	t.Parallel()

	rows := observingGroups(acrux(), eyeResolution)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want Acrux as one", len(rows))
	}

	row := rows[0]
	if row.Target.ID != "* alf01 Cru" {
		t.Errorf("row is %q, want the brighter star's", row.Target.ID)
	}

	if math.Abs(row.ApparentMag-0.667096) > 1e-6 {
		t.Errorf("combined magnitude = %.6f, want 0.667096", row.ApparentMag)
	}

	if want := []string{"* alf01 Cru", "* alf02 Cru"}; !slices.Equal(memberIDs(row), want) {
		t.Errorf("members = %q, want %q", memberIDs(row), want)
	}
}

// TestEqualStarsAreBrighterByTheLogOfTheirCount: n unresolved stars of one
// magnitude m are one point of magnitude m - 2.5 log10(n), whatever m is.
func TestEqualStarsAreBrighterByTheLogOfTheirCount(t *testing.T) {
	t.Parallel()

	for _, n := range []int{2, 3, 4} {
		var objs []placedObject
		for i := range n {
			objs = append(objs, star(string(rune('a'+i)), 10, 20+float64(i)*5/3600.0, 3))
		}

		rows := observingGroups(objs, eyeResolution)
		if len(rows) != 1 {
			t.Fatalf("%d stars 5\" apart in a line: got %d rows, want one", n, len(rows))
		}

		if want := 3 - 2.5*math.Log10(float64(n)); math.Abs(rows[0].ApparentMag-want) > 1e-9 {
			t.Errorf("%d stars of magnitude 3: combined %.9f, want %.9f", n, rows[0].ApparentMag, want)
		}
	}
}

// TestStarsTheObserverResolvesAreRowsOfTheirOwn: two stars 2' apart are two
// points to the naked eye, and each is its own row, with no members.
func TestStarsTheObserverResolvesAreRowsOfTheirOwn(t *testing.T) {
	t.Parallel()

	rows := observingGroups([]placedObject{star("a", 10, 20, 2), star("b", 10, 20+2.0/60, 3)}, eyeResolution)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want two", len(rows))
	}

	for _, r := range rows {
		if r.Members != nil {
			t.Errorf("%q has members %q, want none", r.Target.ID, memberIDs(r))
		}
	}
}

// TestATelescopeSplitsTheSystem: a telescope resolving 1" sees alf Cen A and
// B as two. The system's record, a tenth of an arcsecond from A, holds B's
// light, which B's own row shows, so it is not counted: A's row is at A's own
// magnitude, with the system among its members, and B is a row of its own.
func TestATelescopeSplitsTheSystem(t *testing.T) {
	t.Parallel()

	rows := observingGroups(alfCen(), angle.Arcsec(1))
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want A's and B's", len(rows))
	}

	a := rowByID(t, rows, "* alf Cen A")
	if a.ApparentMag != 0.01 {
		t.Errorf("A's row is at %v, want A's own 0.01: the system's light is A's and B's", a.ApparentMag)
	}

	if want := []string{"* alf Cen", "* alf Cen A"}; !slices.Equal(memberIDs(a), want) {
		t.Errorf("A's members = %q, want %q", memberIDs(a), want)
	}

	if b := rowByID(t, rows, "* alf Cen B"); b.ApparentMag != 1.33 || b.Members != nil {
		t.Errorf("B's row is at %v with members %q, want 1.33 alone", b.ApparentMag, memberIDs(b))
	}
}

// TestASystemWithEveryComponentResolvedJoinsItsBrightestComponent: when the
// telescope resolves even the system's record from A, the record has no
// light of its own to show. It is no row, but a member of A's row, its
// brightest component's, so it can still be reached.
func TestASystemWithEveryComponentResolvedJoinsItsBrightestComponent(t *testing.T) {
	t.Parallel()

	rows := observingGroups(alfCen(), angle.Arcsec(0.01))
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want A's and B's", len(rows))
	}

	a := rowByID(t, rows, "* alf Cen A")
	if a.ApparentMag != 0.01 {
		t.Errorf("A's row is at %v, want A's own 0.01", a.ApparentMag)
	}

	if want := []string{"* alf Cen", "* alf Cen A"}; !slices.Equal(memberIDs(a), want) {
		t.Errorf("A's members = %q, want %q", memberIDs(a), want)
	}
}

// TestZeroResolutionListsEveryStarAlone: with no resolution every object is
// its own row, as VisibleTonight listed them before observing groups.
func TestZeroResolutionListsEveryStarAlone(t *testing.T) {
	t.Parallel()

	objs := append(alfCen(), acrux()...)

	for _, res := range []angle.Angle{0, angle.Arcsec(-1)} {
		rows := observingGroups(objs, res)
		if len(rows) != len(objs) {
			t.Fatalf("resolution %v: got %d rows, want all %d", res, len(rows), len(objs))
		}

		for i, r := range rows {
			if r.Target.ID != objs[i].vo.Target.ID || r.Members != nil {
				t.Errorf("resolution %v: row %d is %q with members %q, want %q alone",
					res, i, r.Target.ID, memberIDs(r), objs[i].vo.Target.ID)
			}
		}
	}
}

// TestOnlyStarsAreGrouped: a galaxy is not a point, and the Moon and the
// planets move against the stars through the night, so a star in front of
// either stays its own row. A double star's record is a point and is grouped.
func TestOnlyStarsAreGrouped(t *testing.T) {
	t.Parallel()

	for _, kind := range []resolve.Kind{resolve.KindGalaxy, resolve.KindNebula, resolve.KindPlanet, resolve.KindMoon, resolve.KindOpenCluster} {
		rows := observingGroups([]placedObject{star("s", 10, 20, 2), placed("o", kind, 10, 20, 3)}, eyeResolution)
		if len(rows) != 2 {
			t.Errorf("a star and a %s at one place: got %d rows, want two", kind, len(rows))
		}
	}

	rows := observingGroups([]placedObject{star("s", 10, 20, 2), placed("d", resolve.KindDoubleStar, 10, 20, 3)}, eyeResolution)
	if len(rows) != 1 {
		t.Errorf("a star and a double star at one place: got %d rows, want one", len(rows))
	}
}

// TestAChainOfUnresolvedStarsIsOnePoint: three stars 50" apart in a line are
// each unresolved from the next, though the ends are 100" apart, and the eye
// sees one point of light.
func TestAChainOfUnresolvedStarsIsOnePoint(t *testing.T) {
	t.Parallel()

	objs := []placedObject{star("a", 10, 20, 3), star("b", 10, 20+50.0/3600, 3), star("c", 10, 20+100.0/3600, 3)}

	if rows := observingGroups(objs, eyeResolution); len(rows) != 1 || len(rows[0].Members) != 3 {
		t.Errorf("got %d rows, want one of three members", len(rows))
	}
}

// TestStarsAcrossTheZeroHourAreGrouped: right ascension wraps at 0h, and two
// stars 7.2" apart across it are as unresolved as any others.
func TestStarsAcrossTheZeroHourAreGrouped(t *testing.T) {
	t.Parallel()

	if rows := observingGroups([]placedObject{star("a", 359.999, 0, 2), star("b", 0.001, 0, 3)}, eyeResolution); len(rows) != 1 {
		t.Errorf("got %d rows, want one", len(rows))
	}
}

// TestAParentIsFoundUnderAnyOfItsIdentifiers: a membership names the parent
// in its catalog's spelling, which need not be the parent record's ID.
// SIMBAD pads its identifiers, and the parent is still the system.
func TestAParentIsFoundUnderAnyOfItsIdentifiers(t *testing.T) {
	t.Parallel()

	objs := alfCen()
	objs[1].vo.Target.MemberOf = []string{"*  alf  Cen"}
	objs[2].vo.Target.MemberOf = []string{"*  alf  Cen"}

	rows := observingGroups(objs, eyeResolution)
	if len(rows) != 1 || rows[0].ApparentMag != -0.1 {
		t.Errorf("got %d rows, the first at %v; want the system's alone at -0.1", len(rows), rows[0].ApparentMag)
	}
}

// TestAClusterIsNoStarsParent: SIMBAD makes 17 Tau a member of the Pleiades,
// Cl Melotte 22. A cluster is no point of light and holds no star's light
// here, so a star that is its member is counted, cluster record or not.
func TestAClusterIsNoStarsParent(t *testing.T) {
	t.Parallel()

	objs := []placedObject{
		star("* 17 Tau", 56.21890, 24.11334, 3.7, "Cl Melotte 22"),
		star("* other", 56.21890, 24.11334+10.0/3600, 3.7),
		placed("Cl Melotte 22", resolve.KindOpenCluster, 56.21890, 24.11334, 1.6),
	}

	rows := observingGroups(objs, eyeResolution)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want the stars' group and the cluster", len(rows))
	}

	if g := rowByID(t, rows, "* 17 Tau"); math.Abs(g.ApparentMag-(3.7-2.5*math.Log10(2))) > 1e-9 {
		t.Errorf("the two stars' row is at %v, want both counted", g.ApparentMag)
	}
}

// TestGroupsDoNotDependOnOrder: the rows, their magnitudes and their members
// are the same whatever order the objects come in.
func TestGroupsDoNotDependOnOrder(t *testing.T) {
	t.Parallel()

	objs := append(alfCen(), acrux()...)
	want := observingGroups(objs, eyeResolution)

	reversed := slices.Clone(objs)
	slices.Reverse(reversed)

	got := observingGroups(reversed, eyeResolution)
	if len(got) != len(want) {
		t.Fatalf("reversed: %d rows, want %d", len(got), len(want))
	}

	for _, w := range want {
		g := rowByID(t, got, w.Target.ID)
		if g.ApparentMag != w.ApparentMag || !slices.Equal(memberIDs(g), memberIDs(w)) {
			t.Errorf("reversed, %q is at %v with %q; want %v with %q",
				w.Target.ID, g.ApparentMag, memberIDs(g), w.ApparentMag, memberIDs(w))
		}
	}
}
