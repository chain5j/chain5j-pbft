// Package core
//
// @author: xwc1125
package core

import "errors"

var (
	errInconsistentSubject = errors.New("inconsistent subjects")               // subject不一致
	errNotFromProposer     = errors.New("message does not come from proposer") // 消息不是来自proposer
	errIgnored             = errors.New("message is ignored")                  // 消息已被忽略
	errFutureMessage       = errors.New("future message")                      // 将来消息
	errInvalidMessage      = errors.New("invalid message")                     // 无效message

	errFailedDecodePrePrepare = errors.New("failed to decode PRE-PREPARE") // 无效的prePrepare消息
	errFailedDecodePrepare    = errors.New("failed to decode PREPARE")     // 无效的prepare消息
	errFailedDecodeCommit     = errors.New("failed to decode COMMIT")      // 无效的commit消息
)
