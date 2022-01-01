// Package core
//
// @author: xwc1125
package core

import (
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	preque "github.com/chain5j/chain5j-pkg/collection/queues/preque"
	"github.com/chain5j/chain5j-pkg/crypto/hashalg/sha3"
	"github.com/chain5j/chain5j-pkg/crypto/signature"
	"github.com/chain5j/chain5j-pkg/event"
	"github.com/chain5j/chain5j-pkg/math"
	"github.com/chain5j/chain5j-pkg/types"
	"github.com/chain5j/chain5j-protocol/models"
	"github.com/chain5j/chain5j-protocol/protocol"
	"github.com/chain5j/logger"
	"github.com/hashicorp/golang-lru"
	"go.uber.org/atomic"
	"math/big"
	"sync"
	"time"
)

const (
	roundChangeTimeout = 20 * time.Second // 轮换的时间为20s
	initRoundTimeout   = 10 * time.Second // 初始化轮换时间10s

	backlogChSize        = 128
	pendingRequestChSize = 16
	localMsgChSize       = 16

	inmemoryPeers    = 40   // recentMessages的缓存个数
	inmemoryMessages = 1024 // 每个peer所缓存的msg个数

	p2pMsgRecvQueue = 1024 // p2p消息的接收队列
)

var (
	_ pbftProtocol.PBFTEngine = new(core)
)

// timeoutEvent 超时事件
type timeoutEvent struct{}

type core struct {
	log     logger.Logger
	peerId  string // 当前validator标识
	state   pbftProtocol.State
	backend pbftProtocol.PBFTBackend
	valSet  pbftProtocol.ValidatorSet

	broadcaster protocol.Broadcaster      // 消息广播
	p2pMsgCh    <-chan *models.P2PMessage // 消息订阅chan
	p2pMsgSub   event.Subscription        // 消息订阅

	// 积压消息
	backlogsLock *sync.Mutex
	backlogs     map[pbftProtocol.Validator]*preque.Prque
	backlogCh    chan backlogEvent

	timeoutCh chan timeoutEvent // 超时通道

	currentRoundState     *roundState // 当前轮换状态
	waitingForRoundChange atomic.Bool // 等待轮换
	roundChangeSet        *roundChangeSet
	roundChangeTimer      *time.Timer

	// 验证回调
	validateFn func(data []byte, signResult *signature.SignResult) (signer string, err error)

	handlerWg *sync.WaitGroup

	requestCh         chan *pbftProtocol.Request
	pendingRequests   *preque.Prque
	pendingRequestsMu *sync.Mutex

	finalCommitCh chan struct{}

	localMsgCh chan []byte // 处理本地消息

	consensusTimestamp time.Time // round 起始时间

	peersMessages *lru.ARCCache // peerId-->lru.NewARC(hash-->bool)
	knownMessages *lru.ARCCache // 当前peer所知的hash(hash-->bool)

	quitCh chan struct{}
}

// NewEngine 创建pbft 引擎
func NewEngine(backend pbftProtocol.PBFTBackend, broadcaster protocol.Broadcaster) pbftProtocol.PBFTEngine {
	log := logger.New("pbft.core", "validator", backend.ID())
	peersMessages, _ := lru.NewARC(inmemoryPeers)
	knownMessages, _ := lru.NewARC(inmemoryMessages)

	p2pMsgCh := make(chan *models.P2PMessage, p2pMsgRecvQueue)
	p2pMsgSub := event.NewSubscription(func(producer <-chan struct{}) error {
		return nil
	})
	if broadcaster != nil {
		p2pMsgSub = broadcaster.SubscribeMsg(models.ConsensusMsg, p2pMsgCh)
	}

	c := &core{
		log:    log,
		peerId: backend.ID(),

		state:   pbftProtocol.StateAcceptRequest,
		backend: backend,

		broadcaster: broadcaster,
		p2pMsgCh:    p2pMsgCh,
		p2pMsgSub:   p2pMsgSub,

		backlogCh:          make(chan backlogEvent, backlogChSize),
		backlogs:           make(map[pbftProtocol.Validator]*preque.Prque),
		backlogsLock:       new(sync.Mutex),
		timeoutCh:          make(chan timeoutEvent, 1),
		handlerWg:          new(sync.WaitGroup),
		requestCh:          make(chan *pbftProtocol.Request, pendingRequestChSize),
		finalCommitCh:      make(chan struct{}, 1),
		pendingRequests:    preque.New(),
		pendingRequestsMu:  new(sync.Mutex),
		consensusTimestamp: time.Time{},
		localMsgCh:         make(chan []byte, localMsgChSize),

		peersMessages: peersMessages,
		knownMessages: knownMessages,
		quitCh:        make(chan struct{}),
	}
	c.validateFn = c.checkValidatorSignature

	return c
}

// Start 实现 core.Engine.Start
func (c *core) Start() error {
	// 开启一个新的轮询。sequence = last sequence + 1
	c.StartNewRound(math.Big0)

	go c.handleLoop()

	return nil
}

// Stop 实现 core.Engine.Stop
func (c *core) Stop() error {
	c.stopTimer()

	go func() {
		c.quitCh <- struct{}{}
	}()

	// 确保所有的协程推出
	c.handlerWg.Wait()

	return nil
}

// handleLoop 处理协程
func (c *core) handleLoop() {
	// 清理状态
	defer func() {
		c.currentRoundState = nil
		c.handlerWg.Done()
	}()

	c.handlerWg.Add(1)

	for {
		select {
		case r := <-c.requestCh: // 处理打包请求
			err := c.HandleRequest(r)
			if err == errFutureMessage {
				c.StoreRequestMsg(r)
			}
		case p := <-c.localMsgCh: // 本地消息(pre_prepare,prepare,commit)
			msg, val, err := c.decodeMsg(p)
			if err == nil {
				c.handleMsg(msg, val)
			}
		case b := <-c.backlogCh: // 积压消息处理
			c.handleMsg(b.msg, b.val)
		case <-c.finalCommitCh: // 最终的commit
			c.HandleFinalCommitted()
		case p2pMsg := <-c.p2pMsgCh: // p2p订阅来的消息
			msg, val, err := c.decodeMsg(p2pMsg.Data)
			if err != nil {
				continue
			}
			hash := types.BytesToHash(sha3.Keccak256(p2pMsg.Data))
			// Mark peer's Message
			ms, ok := c.peersMessages.Get(val.ID())
			var m *lru.ARCCache
			if ok {
				m, _ = ms.(*lru.ARCCache)
			} else {
				m, _ = lru.NewARC(inmemoryMessages)
				c.peersMessages.Add(val.ID(), m)
			}
			m.Add(hash, true)

			if _, ok := c.knownMessages.Get(hash); ok {
				// 消息已经处理
				continue
			}
			c.knownMessages.Add(hash, true)

			if err := c.handleMsg(msg, val); err == nil {
				c.GossipPayload(p2pMsg.Data)
			}
		//case err := <-c.p2pMsgSub.Err():
		//	// p2p订阅消息出错
		//	c.log.Error("pbft core subscribe msg err", "err", err)
		case <-c.timeoutCh: // 超时处理
			c.handleTimeoutMsg()
		case <-c.quitCh: // 退出
			return
		}
	}
}

// handleMsg 处理消息
func (c *core) handleMsg(msg *pbftProtocol.Message, val pbftProtocol.Validator) error {
	log := c.log.New("", "id", c.peerId, "from", val)

	// 保存未来的消息
	testBacklog := func(err error) error {
		if err == errFutureMessage {
			c.StoreBacklog(msg, val)
		}
		return err
	}

	switch msg.Code {
	case pbftProtocol.MsgPrePrepare:
		return testBacklog(c.HandlePrePrepare(msg, val))
	case pbftProtocol.MsgPrepare:
		return testBacklog(c.HandlePrepare(msg, val))
	case pbftProtocol.MsgCommit:
		return testBacklog(c.HandleCommit(msg, val))
	case pbftProtocol.MsgRoundChange:
		return testBacklog(c.HandleRoundChange(msg, val))
	default:
		log.Error("Invalid Message", "msg", msg)
	}

	return errInvalidMessage
}

// handleTimeoutMsg 超时处理
func (c *core) handleTimeoutMsg() {
	if c.currentRoundState != nil {
		c.currentRoundState.UnlockHash()
		c.currentRoundState.pendingRequest = nil
	}

	// If we're not waiting for round change yet, we can try to catch up
	// the max round with F+1 round change Message. We only need to catch up
	// if the max round is larger than currentRoundState round.
	if !c.waitingForRoundChange.Load() {
		maxRound := c.roundChangeSet.MaxRound(c.valSet.FaultTolerantNum() + 1)
		if maxRound != nil && maxRound.Cmp(c.currentRoundState.Round()) > 0 {
			c.log.Trace("round change timeout, send round change", "maxRound", maxRound)
			c.sendRoundChange(maxRound)
			return
		}
	}

	lastProposal, _ := c.backend.LastProposal()
	if lastProposal != nil && lastProposal.Height() >= c.currentRoundState.Sequence() {
		c.log.Trace("round change timeout, catch up latest sequence", "height", lastProposal.Height())
		c.StartNewRound(math.Big0)
	} else {
		c.sendNextRoundChange()
	}
}

// RequestTimeout 请求超时
func (c *core) RequestTimeout() {
	if c.waitingForRoundChange.Load() {
		return
	}
	c.stopTimer()
	c.sendTimeoutEvent()
}

// currentView 当前view
func (c *core) currentView() *pbftProtocol.View {
	return &pbftProtocol.View{
		Sequence: big.NewInt(int64(c.currentRoundState.Sequence())),
		Round:    new(big.Int).Set(c.currentRoundState.Round()),
	}
}

// isProposer 判断当前peer是否为proposer
func (c *core) isProposer() bool {
	v := c.valSet
	if v == nil {
		return false
	}
	return v.IsProposer(c.peerId)
}

func (c *core) sendTimeoutEvent() {
	c.timeoutCh <- timeoutEvent{}
}

// checkValidatorSignature todo 校验签名，并获取签名者peerId
func (c *core) checkValidatorSignature(data []byte, signResult *signature.SignResult) (signer string, err error) {
	return "", nil
}

func (c *core) NewChainHead() error {
	c.FinalCommit()
	return nil
}
