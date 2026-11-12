package advisor

import "strings"

// price is a model's prices, in US dollars a million tokens: input, output,
// writing to the cache (five minutes), and reading from it.
type price struct{ in, out, write, read float64 }

// prices are those of the models a review asks, and those a refused
// request may fall back to (0205). Prompt caching writes at 1.25 times
// the input price.
var prices = map[string]price{
	"claude-opus-5-5":   {in: 4, out: 20, write: 5, read: 0.20},
	"claude-sonnet-5-5": {in: 2, out: 10, write: 2.5, read: 0.20},
	"claude-opus-5":     {in: 5, out: 25, write: 6.25, read: 0.50},
	"claude-opus-4-8":   {in: 5, out: 25, write: 6.25, read: 0.50},
	"claude-sonnet-5":   {in: 2, out: 10, write: 2.5, read: 0.20},
}

// searchPrice is what one web search costs: $10 a thousand.
const searchPrice = 0.01

// priceOf is a model's prices: a model not known is priced as the dearest
// Opus, so a review never thinks it spent less than it did.
func priceOf(model string) price {
	if p, ok := prices[model]; ok {
		return p
	}
	for name, p := range prices {
		if strings.HasPrefix(model, name) {
			return p
		}
	}
	return price{in: 5, out: 25, write: 6.25, read: 0.50}
}

// spend is the tokens and searches of a review so far, and what they cost.
type spend struct {
	requests                   int
	input, output, read, write int64
	searches                   int64
	cost                       float64
}

// add counts one answer's usage, at the prices of the model that gave it.
func (s *spend) add(model string, input, output, read, write, searches int64) {
	p := priceOf(model)
	s.requests++
	s.input += input
	s.output += output
	s.read += read
	s.write += write
	s.searches += searches
	s.cost += (float64(input)*p.in + float64(output)*p.out + float64(read)*p.read + float64(write)*p.write) / 1e6
	s.cost += float64(searches) * searchPrice
}

// next is the most the next request may cost: what the conversation so
// far costs to read from the cache, what came since to write to it, a
// whole answer of maxOut tokens, and every search it may make.
func next(model string, prompt, since, maxOut int64, searches int) float64 {
	p := priceOf(model)
	return (float64(prompt)*p.read+float64(since)*p.write+float64(maxOut)*p.out)/1e6 + float64(searches)*searchPrice
}
