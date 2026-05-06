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
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
)

const mastAPI = "https://mast.stsci.edu/api/v0/invoke"

type Provider struct {
	client *resolve.Client
	cache  resolve.Cache
}

func New() *Provider {
	return &Provider{
		client: resolve.NewClient(),
		cache:  resolve.NewMapCache(),
	}
}

func (p *Provider) Name() string { return "mast" }

func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapObjectResolution, resolve.CapConeSearch}
}

func (p *Provider) Resolve(query string) (resolve.Target, bool) {
	targets := p.Search(query)
	if len(targets) > 0 {
		return targets[0], true
	}
	return resolve.Target{}, false
}

func (p *Provider) Search(query string) []resolve.Target {
	ctx := context.TODO()
	req := resolve.ObjectRequest{Query: query, Limit: 10}

	iter := p.ResolveObject(ctx, req)
	var targets []resolve.Target
	iter(func(t resolve.Target, err error) bool {
		if err == nil {
			targets = append(targets, t)
		}
		return len(targets) < 10
	})
	return targets
}

func (p *Provider) ResolveObject(ctx context.Context, req resolve.ObjectRequest) resolve.SeqIterator[resolve.Target] {
	cacheKey := "resolve:mast:" + resolve.Normalize(req.Query)
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
		return resolve.SliceSeq([]resolve.Target{})
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return func(yield func(resolve.Target, error) bool) {
		resp, err := p.client.Do(httpReq)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}
		defer resp.Body.Close()

		b, err := io.ReadAll(resp.Body)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		var targets []resolve.Target

		if len(b) > 0 && b[0] == '{' {
			// Try parsing as JSON first
			var jsonPayload struct {
				Status             string `json:"status"`
				Msg                string `json:"msg"`
				ResolvedCoordinate []struct {
					CanonicalName string  `json:"canonicalName"`
					RA            float64 `json:"ra"`
					Decl          float64 `json:"decl"`
					Resolver      string  `json:"resolver"`
				} `json:"resolvedCoordinate"`
			}

			if err := json.Unmarshal(b, &jsonPayload); err == nil {
				if jsonPayload.Status == "ERROR" {
					yield(resolve.Target{}, fmt.Errorf("mast: %s", jsonPayload.Msg))
					return
				}
				for _, match := range jsonPayload.ResolvedCoordinate {
					targets = append(targets, resolve.Target{
						ID:      match.CanonicalName,
						Name:    match.CanonicalName,
						Coord:   coord.NewICRS(angle.Deg(match.RA), angle.Deg(match.Decl)),
						Catalog: match.Resolver,
					})
				}
			}
		}

		if len(targets) == 0 {
			// Fallback to XML
			var xmlPayload struct {
				XMLName            xml.Name `xml:"resolvedItems"`
				ResolvedCoordinate []struct {
					CanonicalName string  `xml:"canonicalName"`
					RA            float64 `xml:"ra"`
					Decl          float64 `xml:"dec"` // XML uses 'dec', JSON uses 'decl'
					Resolver      string  `xml:"resolver"`
				} `xml:"resolvedCoordinate"`
			}

			if err := xml.Unmarshal(b, &xmlPayload); err != nil {
				yield(resolve.Target{}, err)
				return
			}

			for _, match := range xmlPayload.ResolvedCoordinate {
				targets = append(targets, resolve.Target{
					ID:      match.CanonicalName,
					Name:    match.CanonicalName,
					Coord:   coord.NewICRS(angle.Deg(match.RA), angle.Deg(match.Decl)),
					Catalog: match.Resolver,
				})
			}
		}

		if err := p.cache.Set(cacheKey, targets); err != nil {
			yield(resolve.Target{}, err)
			return
		}

		for _, t := range targets {
			if !yield(t, nil) {
				return
			}
		}
	}
}

func (p *Provider) ConeSearch(ctx context.Context, req resolve.ConeRequest) resolve.SeqIterator[resolve.Target] {
	// Minimal stub for CAOM spatial search
	return resolve.SliceSeq([]resolve.Target{})
}
