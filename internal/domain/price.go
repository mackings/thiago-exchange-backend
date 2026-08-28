package domain

import "context"

// PriceFeed resolves a reference price (fiat per unit of asset) used for
// floating-rate ads. Implemented by infra/bybit against Bybit's public
// market-data API.
type PriceFeed interface {
	ReferencePrice(ctx context.Context, asset, fiat string) (float64, error)
}
