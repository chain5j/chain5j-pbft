// Package validator
//
// @author: xwc1125
package validator

import (
	"github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pkg/crypto/signature"
	"github.com/chain5j/logger"
	"github.com/chain5j/logger/zap"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

var (
	id1 = "QmTdoWeaoBGywcKWd7ZcCy2vdqeibfN7L38EFGx9GGVkA9"
	id2 = "QmVsiLiCiDsH3RXwz4UkML5ja3QMDo8DPER5nWoGedCFk8"
)

func init() {
	zap.InitWithConfig(&logger.LogConfig{
		Console: logger.ConsoleLogConfig{
			Level:    4,
			Modules:  "*",
			ShowPath: false,
			Format:   "",
			UseColor: true,
			Console:  true,
		},
		File: logger.FileLogConfig{},
	})
}

func TestValidatorSet(t *testing.T) {
	testNewValidatorSet(t)
	testNormalValSet(t)
	testEmptyValSet(t)
	testStickyProposer(t)
	testAddAndRemoveValidator(t)
}

func testNewValidatorSet(t *testing.T) {
	var validators []string
	const ValCnt = 100

	// 创建固定的地址
	for i := 0; i < ValCnt; i++ {
		priv, _ := signature.GenerateKeyWithECDSA(signature.P256)
		addr := signature.PubkeyToAddress(&priv.PublicKey)

		validators = append(validators, addr.Hex())
	}

	// 创建ValidatorSet
	valSet := newDefaultSet(validators, protocol.RoundRobin)
	if valSet == nil {
		t.Errorf("the validator byte array cannot be parsed")
		t.FailNow()
	}

	// 检测排序
	for i := 0; i < ValCnt-1; i++ {
		val := valSet.GetByIndex(uint64(i))
		nextVal := valSet.GetByIndex(uint64(i + 1))
		if strings.Compare(val.String(), nextVal.String()) >= 0 {
			t.Errorf("validator set is not sorted in ascending order")
		}
	}
}

func testNormalValSet(t *testing.T) {
	val1 := &defaultValidator{
		id: id1,
	}
	val2 := &defaultValidator{
		id: id2,
	}

	valSet := newDefaultSet([]string{id1, id2}, protocol.RoundRobin)
	if valSet == nil {
		t.Errorf("the format of validator set is invalid")
		t.FailNow()
	}

	// 检测size
	if size := valSet.Size(); size != 2 {
		t.Errorf("the size of validator set is wrong: have %v, want 2", size)
	}
	// 根据index获取
	if val := valSet.GetByIndex(uint64(0)); !reflect.DeepEqual(val, val1) {
		t.Errorf("validator mismatch: have %v, want %v", val, val1)
	}
	// 测试无效的index
	if val := valSet.GetByIndex(uint64(2)); val != nil {
		t.Errorf("validator mismatch: have %v, want nil", val)
	}
	// 根据ID获取验证者
	if _, val := valSet.GetById(id2); !reflect.DeepEqual(val, val2) {
		t.Errorf("validator mismatch: have %v, want %v", val, val2)
	}
	// 测试无效的ID
	invalidId := "QmTdoWeaoBGywcKWd7ZcCy2vdqeibfN7L38EFGx9GGAAAA"
	if _, val := valSet.GetById(invalidId); val != nil {
		t.Errorf("validator mismatch: have %v, want nil", val)
	}
	// 测试获取提议者
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val1) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val1)
	}
	// 测试提议计算
	lastProposer := id1
	valSet.CalcProposer(lastProposer, uint64(0))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val2) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val2)
	}
	valSet.CalcProposer(lastProposer, uint64(3))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val1) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val1)
	}
	// 测试空的提议者
	lastProposer = ""
	valSet.CalcProposer(lastProposer, uint64(3))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val2) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val2)
	}
}

func testEmptyValSet(t *testing.T) {
	valSet := NewSet([]string{}, protocol.RoundRobin)
	if valSet == nil {
		t.Errorf("validator set should not be nil")
	}
}

func testStickyProposer(t *testing.T) {
	valSet := newDefaultSet([]string{id1, id2}, protocol.Sticky)
	v1 := &defaultValidator{id: id1}
	v2 := &defaultValidator{id: id2}

	// 测试获取提议者
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, v1) {
		t.Errorf("proposer mismatch: have %v, want %v", val, v1)
	}
	// 测试计算提议
	lastProposer := id1
	valSet.CalcProposer(lastProposer, uint64(0))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, v1) {
		t.Errorf("proposer mismatch: have %v, want %v", val, v1)
	}

	valSet.CalcProposer(lastProposer, uint64(1))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, v2) {
		t.Errorf("proposer mismatch: have %v, want %v", val, v1)
	}
	// 测试空提议者
	lastProposer = ""
	valSet.CalcProposer(lastProposer, uint64(3))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, v2) {
		t.Errorf("proposer mismatch: have %v, want %v", val, v2)
	}
}

func testAddAndRemoveValidator(t *testing.T) {
	valSet := NewSet([]string{}, protocol.RoundRobin)
	if !valSet.AddValidator("2") {
		t.Error("the validator should be added")
	}
	if valSet.AddValidator("2") {
		t.Error("the existing validator should not be added")
	}
	valSet.AddValidator("1")
	valSet.AddValidator("0")
	if len(valSet.List()) != 3 {
		t.Error("the size of validator set should be 3")
	}

	for i, v := range valSet.List() {
		expected := strconv.Itoa(i)
		if v.ID() != expected {
			t.Errorf("the order of validators is wrong: have %v, want %v", v.ID(), expected)
		}
	}

	if !valSet.RemoveValidator("2") {
		t.Error("the validator should be removed")
	}
	if valSet.RemoveValidator("2") {
		t.Error("the non-existing validator should not be removed")
	}
	if len(valSet.List()) != 2 {
		t.Error("the size of validator set should be 2")
	}
	valSet.RemoveValidator("1")
	if len(valSet.List()) != 1 {
		t.Error("the size of validator set should be 1")
	}
	valSet.RemoveValidator("0")
	if len(valSet.List()) != 0 {
		t.Error("the size of validator set should be 0")
	}
}
