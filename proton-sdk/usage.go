package protonsdk

import "fmt"

type Usage struct {
	InputTokens       int64
	OutputTokens      int64
	TotalTokens       int64
	CachedInputTokens int64
}

func (u Usage) Validate() error {
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.TotalTokens < 0 || u.CachedInputTokens < 0 {
		return fmt.Errorf("%w: token usage cannot be negative", ErrInvalidEvent)
	}
	return nil
}
