package remote

// rejection is a source version whose copy was refused, by a [WithRead] read
// or a [WithValidate] check: the ETag its source reported when it was fetched,
// and what the refusal said.
type rejection struct {
	sourceETag string
	cause      string
}

// rejectionKey names a cached object across every data directory this client
// is pointed at in its lifetime. The cache key alone would let a rejection made
// against one cache refuse a fetch into another.
func (c *Client) rejectionKey(cacheKey string) string {
	return c.DataDirURL() + "\x00" + cacheKey
}

// reject records that cacheKey's source version sourceETag was refused for
// cause. A version with no ETag cannot be recognized again, so it is not
// recorded and the next call fetches it again.
func (c *Client) reject(cacheKey, sourceETag string, cause error) {
	if sourceETag == "" {
		return
	}

	key := c.rejectionKey(cacheKey)

	c.rejectedMu.Lock()
	defer c.rejectedMu.Unlock()

	if c.rejected == nil {
		c.rejected = make(map[string]rejection)
	}

	c.rejected[key] = rejection{sourceETag: sourceETag, cause: cause.Error()}
}

// rejection reports whether cacheKey's source version sourceETag is one this
// client refused, and what the refusal said.
func (c *Client) rejection(cacheKey, sourceETag string) (rejection, bool) {
	if sourceETag == "" {
		return rejection{}, false
	}

	key := c.rejectionKey(cacheKey)

	c.rejectedMu.Lock()
	defer c.rejectedMu.Unlock()

	r, ok := c.rejected[key]

	return r, ok && r.sourceETag == sourceETag
}

// forgetRejection drops what was recorded against cacheKey, once a different
// version of it has been fetched.
func (c *Client) forgetRejection(cacheKey string) {
	key := c.rejectionKey(cacheKey)

	c.rejectedMu.Lock()
	defer c.rejectedMu.Unlock()

	delete(c.rejected, key)
}
