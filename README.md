# chain5j-pbft

## 简介
`chain5j-pbft` chain5j链pbft共识。

## 功能
一致性的确保主要分为这三个阶段：

```
预准备（pre-prepare）：
准备(prepare)：
确认(commit)：
```

```
1. Request：请求端C发送请求到任意一节点，这里是0
2. Pre-Prepare：服务端0收到C的请求后进行广播，扩散至123
3. Prepare：123,收到后记录并再次广播，1->023，2->013，3因为宕机无法广播
4. Commit：0123节点在Prepare阶段，若收到超过一定数量的相同请求，则进入Commit阶段，广播Commit请求
5. Reply：0123节点在Commit阶段，若收到超过一定数量的相同请求，则对C进行反馈
```

## Consensus通用部分

### New

1. 从 `nodeKey`中获取节点的ID
2. 从 `config`中获取链配置
3. 判断链配置中共识配置是否符合要求，并将共识配置信息从map中解析到对象上
4. 创建 `core.NewEngine` 创建pbft的核心逻辑

## Start

1. 启动pbft核心逻辑
2. 调用pbft核心函数，NewChainHead，创建新的链头

## Begin

1. 调用pbft核心函数，NewChainHead，创建新的链头

## Prepare

1. 预处理`header`，从`blockReader`中读取父区块`header`
2. 从`snapshot`中获取父区块的快照
3. 从`voteManager`中获取`managerVote`和`validatorVote`的数据
4. 设置`header.Consensus`和`header.Timestamp`的值

## VerifyHeader（header已经被签名）

1. 验证`header.Height`是否为创世高度，如果是创世高度，那么就返回错误
2. 判断`header.Timestamp`是否超过可容忍的时间范围。此操作避免服务器之间时间不同步
3. 将`header.Consensus`转换为共识所需的参数`ConsensusData`
4. 验证`header`中的其他字段
    1. 如果是创世区块，直接返回nil
    2. 判断父区块`parentHeader.Height ?= header.Height-1`，判断`parentHeader.Hash ?= header.ParentHash`
    3. 判断父区块和当前区块的时间戳是否相隔太近
    4. 验证`header`签名是否正确，并获取签名者
    5. 判断签名者是否是`validator`

## Finalize

1. 根据`header`和`txs`进行`block`的最终组装
2. 如果涉及挖矿激励也在此方法中进行

## Seal

1. 从`snapshot` 中获取父区块的快照
2. 判断当前节点是否为快照中的验证者
3. 从`blockReader`中获取父区块`header`
4. 更新区块时间戳和区块签名
5. 锁定提案`Hash`，需要加锁处理
6. 使用`pbft.core`进行`request`请求
7. 线程处理结果
    1. 清空`commit`通道
    2. 等待`pbft.core`的`commit`通知
    3. 判断通知结果和提案区块`hash`是否一致性
    4. 如果`hash`一致，那么会将提案`block`通过`chan`返回给上一层，做入库处理
    5. 如果`hash`不一致，那么反馈给上层为`nil`
    6. 将提案区块`hash`设置为空，并释放锁

## PBFT.Core

### New

1. 通过`broadcaster`订阅P2P的共识消息
2. 获取当前节点的`peerId`

### Start
1. 调用`StartNewRound`开启新的轮换
   1. 获取最新的提案
   2. 判断共识时间戳
   3. 判断是否进行轮询切换
   4. 创建新的`view`视图
   5. 清除无效轮换信息
   6. 创建新的轮换快照
   7. 根据最新的提案集轮询次数计算出最新的提案者
   8. 设置可接收请求状态
   9. 如果当前节点是`proposer`，那么将会发起`pre-prepare`
2. 启动协程，监听通道数据
   1. 监听`requestCh`
      1. `HandleRequest`处理请求的数据
         1. 验证请求数据的正确性
         2. 判断当前状态，如果是`StateAcceptRequest`，那么将请求作为`pendingRequest`
         3. 如果当前状态是`StateAcceptRequest`，并且不处于等待轮换状态，那么将`SendPrePrepare`
      2. 如果处理没有错误，那么保存请求数据
   2. 监听`localMsgCh`
      1. `decodeMsg`解析数据
      2. `handleMsg`处理
   3. 监听`backlogCh`
      1. `handleMsg`处理
   4. 监听`finalCommitCh`
      1. `HandleFinalCommitted`处理
   5. 监听`p2pMsgCh`
      1. `decodeMsg`解析数据
      2. 获取`p2pMsg.Data`的`Hash`
      3. 标记消息来源
      4. `handleMsg`处理
      5. 广播`p2pMsg.Data`
   6. 监听`timeoutCh`
      1. `handleTimeoutMsg`处理


## 证书
`chain5j-pbft` 的源码允许用户在遵循 [Apache 2.0 开源证书](LICENSE) 规则的前提下使用。

## 版权
Copyright@2022 chain5j

![chain5j](./chain5j.png)

