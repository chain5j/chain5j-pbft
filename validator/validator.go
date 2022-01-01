// Package validator
//
// @author: xwc1125
package validator

import pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"

var (
	_ pbftProtocol.Validator = new(defaultValidator)
)

// defaultValidator  pbft.Validator的实现
type defaultValidator struct {
	id string
}

func (val *defaultValidator) ID() string {
	return val.id
}

func (val *defaultValidator) String() string {
	return val.id
}
