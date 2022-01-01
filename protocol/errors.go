// Package protocol
//
// @author: xwc1125
package protocol

import "errors"

var (
	ErrUnauthorized    = errors.New("unauthorized validator") // validator未授权
	ErrStoppedEngine   = errors.New("stopped engine")         // 共识已停止
	ErrStartedEngine   = errors.New("started engine")         // 共识已经启动，再次启动时会报错
	ErrOldMessage      = errors.New("old message")            // pbft旧消息
	ErrUnknownAncestor = errors.New("unknown ancestor")       // 校验新区块时，祖先区块未知
	ErrFutureBlock     = errors.New("block in the future")    // 当区块的时间戳大于当前节点的时间戳
)
