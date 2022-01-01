// Package protocol
//
// @author: xwc1125
package protocol

import (
	"bytes"
	"github.com/chain5j/chain5j-pkg/crypto/signature"
	"github.com/chain5j/chain5j-pkg/types"
	"github.com/chain5j/chain5j-protocol/protocol"
)

// PrepareCommittedSeal 根据hash返回committed的签名消息内容
func PrepareCommittedSeal(hash types.Hash) []byte {
	var buf bytes.Buffer
	buf.Write(hash.Bytes())
	buf.Write([]byte("commit"))
	return buf.Bytes()
}

// GetSignatureValidator 从签名数据中获取签名地址
func GetSignatureValidator(nodeKey protocol.NodeKey, data []byte, sig *signature.SignResult) (string, error) {
	peerID, err := nodeKey.RecoverId(data, sig)
	if err != nil {
		return "", err
	}
	return peerID.String(), nil
}
