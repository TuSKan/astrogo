package fink

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

const ssoftSingleFixture = `{
	"sso_name": "Benoitcarry",
	"sso_number": 8467,
	"H_1": 15.0,
	"H_2": 14.5,
	"err_H_1": 0.1,
	"err_H_2": 0.1,
	"G1_1": 0.20,
	"G1_2": 0.25,
	"G2_1": 0.40,
	"G2_2": 0.35,
	"R": 0.75,
	"alpha0": 30.0,
	"delta0": 15.0,
	"a_b": 1.2,
	"a_c": 1.5,
	"fit": 0,
	"status": 1,
	"n_obs": 120,
	"rms": 0.05
}`

func TestNewAndNewWithVersion(t *testing.T) {
	p := New()
	if p.client == nil {
		t.Fatal("New() did not set a client")
	}

	if p.version != defaultVersion {
		t.Errorf("New() version = %q, want %q", p.version, defaultVersion)
	}

	p2 := NewWithVersion("2024.01")
	if p2.version != "2024.01" {
		t.Errorf("NewWithVersion version = %q, want %q", p2.version, "2024.01")
	}
}

func TestQuerySingleByNumberAndName(t *testing.T) {
	t.Cleanup(remote.Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(ssoftSingleFixture))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.FINK, srv.URL); err != nil {
		t.Fatal(err)
	}

	p := New()

	rec, err := p.querySingle(context.Background(), 8467, "")
	if err != nil {
		t.Fatalf("querySingle by number: %v", err)
	}

	if rec.Name != "Benoitcarry" || rec.Number != 8467 {
		t.Errorf("querySingle by number = %+v, want Name=Benoitcarry Number=8467", rec)
	}

	rec2, err := p.querySingle(context.Background(), 0, "Benoitcarry")
	if err != nil {
		t.Fatalf("querySingle by name: %v", err)
	}

	if rec2.Name != "Benoitcarry" {
		t.Errorf("querySingle by name = %+v, want Name=Benoitcarry", rec2)
	}

	if _, err := p.querySingle(context.Background(), 0, ""); !errors.Is(err, ErrNoIdentifier) {
		t.Errorf("querySingle with no identifier: err = %v, want ErrNoIdentifier", err)
	}
}

func TestQuerySingleRemoteException(t *testing.T) {
	t.Cleanup(remote.Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"RemoteException": "sso_number not found"}`))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.FINK, srv.URL); err != nil {
		t.Fatal(err)
	}

	p := New()

	if _, err := p.querySingle(context.Background(), 999999999, ""); !errors.Is(err, ErrRemoteException) {
		t.Errorf("err = %v, want ErrRemoteException", err)
	}
}

func TestResolveViaSingleObjectAPI(t *testing.T) {
	t.Cleanup(remote.Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(ssoftSingleFixture))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.FINK, srv.URL); err != nil {
		t.Fatal(err)
	}

	p := New()

	target, ok := p.Resolve("8467")
	if !ok || target.Name != "Benoitcarry" {
		t.Fatalf("Resolve(8467) = %+v, %v, want Benoitcarry, true", target, ok)
	}

	if targets := p.Search("8467"); len(targets) != 1 {
		t.Errorf("Search(8467) = %v, want exactly one target", targets)
	}

	if seq := p.ResolveObject(context.Background(), resolve.ObjectRequest{Query: "8467"}); seq == nil {
		t.Error("ResolveObject returned a nil iterator")
	}
}
