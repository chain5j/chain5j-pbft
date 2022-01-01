// Package pbft
//
// @author: xwc1125
package pbft

import (
	"fmt"
	"github.com/chain5j/chain5j-pkg/codec"
	"github.com/chain5j/chain5j-pkg/util/hexutil"
	"testing"
)

func TestConsensusData(t *testing.T) {
	data := &ConsensusData{
		Validators: []string{
			"QmQYZ7b9mUuDr43zsRE5rd1RWRZXMptWFCUkq8p5B1wnJH",
			"QmWaGEug7hPSNLtTswWZigD5vuLu4zTZUKyaLUJ851sCHk",
		},
	}

	b, _ := codec.DefaultCodec.Encode(data)

	fmt.Println(hexutil.Encode(b))

	var data2 ConsensusData
	err := codec.DefaultCodec.Decode(b, &data2)
	if err != nil {
		t.Fatal(err)
	}
}
