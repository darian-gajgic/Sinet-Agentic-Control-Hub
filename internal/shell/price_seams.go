package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
)

// price_seams.go — the composition-root adapter behind the B6-3A price surface
// (api.PriceSurface).
//
// It exists for the same reason benchmark_verbs.go does: internal/api never
// imports internal/metering (the §38 R8 money scan asserts it), because every
// dollar figure the API serves must be READ from the layer that owns money
// rather than shaped by the transport. The shell is the one place allowed to see
// both vocabularies, and this adapter's whole job is to marshal in one direction
// and unmarshal in the other.
//
// The row body crosses the seam as JSON in BOTH directions. S10.3 owns what a
// row is; the transport treats it as opaque, and this file is the only place
// where the two representations meet.

// priceSeam adapts the composed live table to api.PriceSurface.
type priceSeam struct{ live *metering.LivePriceTable }

// priceSurfaceSeam returns the api seam, or a true nil when no price table is
// composed in this process — which leaves the price routes answering 503 rather
// than serving an empty table that looks like a real one.
func priceSurfaceSeam(live *metering.LivePriceTable) api.PriceSurface {
	if live == nil {
		return nil
	}
	return priceSeam{live: live}
}

func (p priceSeam) PriceRows(ctx context.Context) (json.RawMessage, error) {
	rows, err := p.live.Rows(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return nil, fmt.Errorf("shell: marshal price rows: %w", err)
	}
	return raw, nil
}

func (p priceSeam) TableVersion() string { return p.live.Version() }

// priceRowBody is the wire shape of one row. It is declared HERE rather than in
// internal/api so the transport never names a dollar field, and it maps
// one-to-one onto metering.PriceRow — the field names are the stored column
// names, so what an operator sends is what the table holds.
type priceRowBody struct {
	Model      string `json:"model"`
	Lane       string `json:"lane"`
	UnitPrices struct {
		InputUSD         float64 `json:"input_usd"`
		OutputUSD        float64 `json:"output_usd"`
		CacheReadUSD     float64 `json:"cache_read_usd"`
		CacheCreationUSD float64 `json:"cache_creation_usd"`
		ServerToolUSD    float64 `json:"server_tool_usd"`
	} `json:"unit_prices"`
	EffectiveFrom string `json:"effective_from"`
	VerifiedOn    string `json:"verified_on"`
	Source        string `json:"source"`
}

func (p priceSeam) AddPriceRow(ctx context.Context, actor, reason string, row json.RawMessage) (json.RawMessage, error) {
	var body priceRowBody
	dec := json.NewDecoder(bytes.NewReader(row))
	// Unknown members are refused rather than dropped: a misspelled price field
	// would otherwise store silently as zero, and a zero rate is exactly the
	// figure S10.3's coverage guard exists to keep off a receipt.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		return nil, &api.SurfaceError{Status: http.StatusBadRequest, Code: "bad_request",
			Msg: "price row: " + err.Error()}
	}
	effFrom, err := parseRowDate(body.EffectiveFrom, "effective_from")
	if err != nil {
		return nil, err
	}
	verOn, err := parseRowDate(body.VerifiedOn, "verified_on")
	if err != nil {
		return nil, err
	}
	stored, err := p.live.AddRow(ctx, metering.AddRowRequest{
		Row: metering.PriceRow{
			Model: body.Model, Lane: body.Lane,
			Prices: metering.UnitPrices{
				InputUSD:         body.UnitPrices.InputUSD,
				OutputUSD:        body.UnitPrices.OutputUSD,
				CacheReadUSD:     body.UnitPrices.CacheReadUSD,
				CacheCreationUSD: body.UnitPrices.CacheCreationUSD,
				ServerToolUSD:    body.UnitPrices.ServerToolUSD,
			},
			EffectiveFrom: effFrom, VerifiedOn: verOn, Source: body.Source,
		},
		Actor: actor, Reason: reason,
	})
	if err != nil {
		return nil, priceStatus(err)
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("shell: marshal stored price row: %w", err)
	}
	return raw, nil
}

// parseRowDate reads one of the row's two dates. Both are required and both are
// the caller's input, so a bad one is a 400 naming the field — not a platform
// fault (the §39-B drain-D6 principle).
func parseRowDate(raw, field string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, &api.SurfaceError{Status: http.StatusBadRequest, Code: "bad_request",
			Msg: fmt.Sprintf("price row: %q is required (S10.3 rows are effective-dated and record when they were verified)", field)}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, &api.SurfaceError{Status: http.StatusBadRequest, Code: "bad_request",
			Msg: fmt.Sprintf("price row: bad %s %q: want an RFC3339 timestamp", field, raw)}
	}
	return t.UTC(), nil
}

// priceStatus maps the store's own refusal onto a status. An inadmissible row is
// the caller's input being wrong; anything else passes through as a 500.
func priceStatus(err error) error {
	if errors.Is(err, metering.ErrBadPriceRow) {
		return &api.SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
	}
	return err
}
