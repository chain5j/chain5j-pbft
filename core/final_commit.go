// Package core
//
// @author: xwc1125
package core

import "github.com/chain5j/chain5j-pkg/math"

// FinalCommit 最终提交
func (c *core) FinalCommit() {
	c.log.Debug("final_commit-1) send final Commit message")
	go func() {
		c.finalCommitCh <- struct{}{}
	}()
}

// HandleFinalCommitted 处理最终的
func (c *core) HandleFinalCommitted() error {
	c.log.Debug("final_commit-2) handle final committed", "msg", "Received a final committed proposal")
	c.StartNewRound(math.Big0)
	return nil
}
