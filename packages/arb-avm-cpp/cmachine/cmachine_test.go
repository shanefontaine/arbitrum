/*
* Copyright 2019, Offchain Labs, Inc.
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

package cmachine

import (
	"log"
	"os"
	"testing"
)

func TestMachineCreation(t *testing.T) {
	dePath := "dbPath"

	if err := os.RemoveAll(dePath); err != nil {
		log.Fatal(err)
	}

	valueCache, err := NewValueCache()
	if err != nil {
		t.Fatal(err)
	}

	mach1, err := New(codeFile)
	if err != nil {
		t.Fatal(err)
	}

	checkpointStorage, err := NewCheckpoint("dbPath")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkpointStorage.Initialize(codeFile); err != nil {
		t.Fatal(err)
	}
	defer checkpointStorage.CloseCheckpointStorage()
	mach2, err := checkpointStorage.GetInitialMachine(valueCache)
	if err != nil {
		t.Fatal(err)
	}

	if mach1.Hash() != mach2.Hash() {
		t.Error("Machine hashes not equal")
	}

	if err := os.RemoveAll(dePath); err != nil {
		log.Fatal(err)
	}
}
