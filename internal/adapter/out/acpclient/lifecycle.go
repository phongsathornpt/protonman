package acpclient

// Done is closed when the ACP subprocess transport terminates.
// Callers may use it to supervise and restart the process without coupling to
// the client's internal command lifecycle.
func (c *Client) Done() <-chan struct{} {
	if c == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return c.closed
}
