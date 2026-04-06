package mast

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
)

const mastAPI = "https://mast.stsci.edu/api/v0/invoke"

type Provider struct {
	client *catalog.Client
	cache  catalog.Cache
}

func New() *Provider {
	return &Provider{
		client: catalog.NewClient(),
		cache:  catalog.NewArrowCache(),
	}
}

func (p *Provider) Name() string { return "mast" }

func (p *Provider) Capabilities() []catalog.Capability {
	return []catalog.Capability{catalog.CapObjectResolution, catalog.CapConeSearch}
}

func (p *Provider) Resolve(query string) (catalog.Target, bool) {
	targets := p.Search(query)
	if len(targets) > 0 {
		return targets[0], true
	}
	return catalog.Target{}, false
}

func (p *Provider) Search(query string) []catalog.Target {
	ctx := context.TODO()
	req := catalog.ObjectRequest{Query: query, Limit: 10}
	
	iter := p.ResolveObject(ctx, req)
	var targets []catalog.Target
	iter(func(t catalog.Target, err error) bool {
		if err == nil {
			targets = append(targets, t)
		}
		return len(targets) < 10
	})
	return targets
}

func (p *Provider) ResolveObject(ctx context.Context, req catalog.ObjectRequest) catalog.SeqIterator[catalog.Target] {
	cacheKey := "resolve:mast:" + catalog.Normalize(req.Query)
	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	payload := map[string]interface{}{
		"service": "Mast.Name.Lookup",
		"format":  "json",
		"params": map[string]string{
			"input": req.Query,
		},
	}

	b, _ := json.Marshal(payload)
	v := url.Values{}
	v.Set("request", string(b))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, mastAPI, strings.NewReader(v.Encode()))
	if err != nil {
		return catalog.SliceSeq([]catalog.Target{})
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return func(yield func(catalog.Target, error) bool) {
		resp, err := p.client.Do(httpReq)
		if err != nil {
			yield(catalog.Target{}, err)
			return
		}
		defer resp.Body.Close()

		b, err := io.ReadAll(resp.Body)
		if err != nil {
			yield(catalog.Target{}, err)
			return
		}

		if len(b) > 0 && b[0] == '{' {
			// MAST returns JSON for errors
			var errPayload struct {
				Status string `json:"status"`
				Msg    string `json:"msg"`
			}
			if err := json.Unmarshal(b, &errPayload); err == nil && errPayload.Status == "ERROR" {
				yield(catalog.Target{}, fmt.Errorf("mast: %s", errPayload.Msg))
				return
			}
		}

		var payload struct {
			XMLName            xml.Name `xml:"resolvedItems"`
			ResolvedCoordinate []struct {
				CanonicalName string  `xml:"canonicalName"`
				RA            float64 `xml:"ra"`
				Decl          float64 `xml:"dec"`
				Resolver      string  `xml:"resolver"`
			} `xml:"resolvedCoordinate"`
		}

		if err := xml.Unmarshal(b, &payload); err != nil {
			yield(catalog.Target{}, err)
			return
		}

		var targets []catalog.Target
		for _, match := range payload.ResolvedCoordinate {
			targets = append(targets, catalog.Target{
				ID:      match.CanonicalName,
				Name:    match.CanonicalName,
				Coord:   coord.NewICRS(angle.Deg(match.RA), angle.Deg(match.Decl)),
				Catalog: match.Resolver,
			})
		}

		p.cache.Set(cacheKey, targets)

		for _, t := range targets {
			if !yield(t, nil) {
				return
			}
		}
	}
}

func (p *Provider) ConeSearch(ctx context.Context, req catalog.ConeRequest) catalog.SeqIterator[catalog.Target] {
	// Minimal stub for CAOM spatial search
	return catalog.SliceSeq([]catalog.Target{})
}
