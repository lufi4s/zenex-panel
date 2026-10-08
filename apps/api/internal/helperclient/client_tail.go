package helperclient

import "context"

// Output runs an operation that returns text (for example, a log tail) and
// returns that text.
func (c *Client) Output(ctx context.Context, op string, args map[string]string) (string, error) {
	return c.call(ctx, op, args, func(r response) string { return r.Output })
}
