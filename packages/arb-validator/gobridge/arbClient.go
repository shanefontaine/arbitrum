/*
 * Copyright 2020, Offchain Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gobridge

import (
	"context"
	"errors"
	"fmt"
	"github.com/offchainlabs/arbitrum/packages/arb-validator/structures"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/offchainlabs/arbitrum/packages/arb-util/common"
	"github.com/offchainlabs/arbitrum/packages/arb-validator/arbbridge"
)

var reorgError = errors.New("reorg occured")
var headerRetryDelay = time.Second * 2
var maxFetchAttempts = 5

type GoArbClient struct {
	GoEthClient *goEthdata
}

func NewEthClient(ethURL string) (*GoArbClient, error) {
	// call to goEth.go - getGoEth(ethURL)
	client := GoArbClient{getGoEth(ethURL)}

	return &client, nil
}

func (c *GoArbClient) SubscribeBlockHeaders(ctx context.Context, startBlockId *structures.BlockId) (<-chan arbbridge.MaybeBlockId, error) {
	blockIdChan := make(chan arbbridge.MaybeBlockId, 100)

	blockIdChan <- arbbridge.MaybeBlockId{BlockId: startBlockId}
	prevBlockId := startBlockId
	go func() {
		defer close(blockIdChan)

		for {
			var nextBlock *structures.BlockId
			fetchErrorCount := 0
			for {
				if prevBlockId == nil {
					fmt.Println("prevBlockId nil")
				}
				nextHeight := common.NewTimeBlocks(new(big.Int).Add(prevBlockId.Height.AsInt(), big.NewInt(1)))
				n, notFound := c.GoEthClient.getBlockFromHeight(nextHeight)
				if notFound == nil {
					// Got next header
					nextBlock = n
					break
				}

				select {
				case <-ctx.Done():
					// Getting header must have failed due to context cancellation
					return
				default:
				}

				if notFound != nil {
					log.Printf("Failed to fetch next header on attempt %v", fetchErrorCount)
					fetchErrorCount++
				}

				if fetchErrorCount >= maxFetchAttempts {
					err := fmt.Sprint("Next header not found after ", fetchErrorCount, " attempts")
					blockIdChan <- arbbridge.MaybeBlockId{Err: errors.New(err)}
					return
				}

				// Header was not found so wait before checking again
				time.Sleep(headerRetryDelay)
			}

			if c.GoEthClient.parentHashes[*nextBlock] != prevBlockId.HeaderHash {
				blockIdChan <- arbbridge.MaybeBlockId{Err: reorgError}
				return
			}

			prevBlockId = nextBlock
			blockIdChan <- arbbridge.MaybeBlockId{BlockId: prevBlockId}
		}
	}()

	return blockIdChan, nil
}

func (c *GoArbClient) NewArbFactoryWatcher(address common.Address) (arbbridge.ArbFactoryWatcher, error) {
	return newArbFactoryWatcher(address, c)
}

func (c *GoArbClient) NewRollupWatcher(address common.Address) (arbbridge.ArbRollupWatcher, error) {
	return newRollupWatcher(address, c)
}

func (c *GoArbClient) NewExecutionChallengeWatcher(address common.Address) (arbbridge.ExecutionChallengeWatcher, error) {
	return newExecutionChallengeWatcher(address, c)
}

func (c *GoArbClient) NewMessagesChallengeWatcher(address common.Address) (arbbridge.MessagesChallengeWatcher, error) {
	return newMessagesChallengeWatcher(address, c)
}

func (c *GoArbClient) NewPendingTopChallengeWatcher(address common.Address) (arbbridge.PendingTopChallengeWatcher, error) {
	return newPendingTopChallengeWatcher(address, c)
}

func (c *GoArbClient) NewOneStepProof(address common.Address) (arbbridge.OneStepProof, error) {
	return newOneStepProof(address, c)
}

func (c *GoArbClient) CurrentBlockId(ctx context.Context) (*structures.BlockId, error) {
	return c.GoEthClient.LastMinedBlock, nil
}

func (c *GoArbClient) BlockIdForHeight(ctx context.Context, height *common.TimeBlocks) (*structures.BlockId, error) {
	//if height == nil {panic("nill height")}
	//fmt.Println("blockNumbers", c.GoEthClient.blockNumbers)
	//fmt.Println("height", height)
	//fmt.Println("height.AsInt().Uint64()", height.AsInt().Uint64())
	//fmt.Println("c.GoEthClient.blockNumbers[height.AsInt().Uint64()]", c.GoEthClient.blockNumbers[height])
	block, err := c.GoEthClient.getBlockFromHeight(height)
	if err != nil {
		errstr := fmt.Sprintln("block height", height, " not found")
		return nil, errors.New(errstr)
	}
	return block, nil
}

type TransOpts struct {
	sync.Mutex
	From  common.Address // Ethereum account to send the transaction from
	Nonce *big.Int       // Nonce to use for the transaction execution (nil = use pending state)

	Value    *big.Int // Funds to transfer along along the transaction (nil = 0 = no funds)
	GasPrice *big.Int // Gas price to use for the transaction execution (nil = gas price oracle)
	GasLimit uint64   // Gas limit to set for the transaction execution (0 = estimate)
}

type GoArbAuthClient struct {
	*GoArbClient
	auth *TransOpts
}

func NewEthAuthClient(ethURL string, auth *TransOpts) (*GoArbAuthClient, error) {
	client, err := NewEthClient(ethURL)
	if err != nil {
		return nil, err
	}
	return &GoArbAuthClient{
		GoArbClient: client,
		auth:        auth,
	}, nil
}

func (c *GoArbAuthClient) Address() common.Address {
	return c.auth.From
}

func (c *GoArbAuthClient) NewArbFactory(address common.Address) (arbbridge.ArbFactory, error) {
	return newArbFactory(address, c.GoArbClient)
}

func (c *GoArbAuthClient) NewRollup(address common.Address) (arbbridge.ArbRollup, error) {
	return newRollup(address, c)
}

func (c *GoArbAuthClient) NewPendingInbox(address common.Address) (arbbridge.PendingInbox, error) {
	return newPendingInbox(address, c.GoArbClient)
}

func (c *GoArbAuthClient) NewChallengeFactory(address common.Address) (arbbridge.ChallengeFactory, error) {
	return newChallengeFactory(address, c, c.auth)
}

func (c *GoArbAuthClient) NewExecutionChallenge(address common.Address) (arbbridge.ExecutionChallenge, error) {
	return NewExecutionChallenge(address, c)
}

func (c *GoArbAuthClient) NewMessagesChallenge(address common.Address) (arbbridge.MessagesChallenge, error) {
	return newMessagesChallenge(address, c)
}

func (c *GoArbAuthClient) NewPendingTopChallenge(address common.Address) (arbbridge.PendingTopChallenge, error) {
	return NewPendingTopChallenge(address, c)
}

func (c *GoArbAuthClient) DeployChallengeTest(ctx context.Context, challengeFactory common.Address) (arbbridge.ChallengeTester, error) {
	c.auth.Lock()
	defer c.auth.Unlock()
	//testerAddress, tx, _, err := challengetester.DeployChallengeTester(c.auth.auth, c.client, challengeFactory.ToEthAddress())
	//if err != nil {
	//	return nil, err
	//}
	//if err := waitForReceipt(
	//	ctx,
	//	c.client,
	//	c.auth.auth.From,
	//	tx,
	//	"DeployChallengeTester",
	//); err != nil {
	//	return nil, err
	//}
	tester, err := NewChallengeTester(c)
	if err != nil {
		return nil, err
	}
	return tester, nil
}

func (c *GoArbAuthClient) DeployOneStepProof(ctx context.Context) (arbbridge.OneStepProof, error) {
	c.auth.Lock()
	defer c.auth.Unlock()
	//ospAddress, tx, _, err := executionchallenge.DeployOneStepProof(c.auth.auth, c.client)
	//if err != nil {
	//	return nil, err
	//}
	//if err := waitForReceipt(
	//	ctx,
	//	c.client,
	//	c.auth.auth.From,
	//	tx,
	//	"DeployOneStepProof",
	//); err != nil {
	//	return nil, err
	//}
	//osp, err := c.NewOneStepProof(common.NewAddressFromEth(ospAddress))
	//if err != nil {
	//	return nil, err
	//}
	return nil, nil
}
