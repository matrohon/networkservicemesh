// Copyright (c) 2018 Cisco and/or its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8sclient_test

import (
	"testing"

	"github.com/ligato/networkservicemesh/plugins/k8sclient"
	"github.com/ligato/networkservicemesh/utils/helper/deptools"
	"github.com/ligato/networkservicemesh/utils/registry/testsuites"
	. "github.com/onsi/gomega"
)

func TestNonDefaultName(t *testing.T) {
	RegisterTestingT(t)
	name := "foo"
	plugin := k8sclient.NewPlugin(k8sclient.UseDeps(&k8sclient.Deps{Name: name}))
	Expect(plugin).NotTo(BeNil())
	Expect(deptools.Check(plugin)).To(Succeed())
}

func TestWithRegistry(t *testing.T) {
	name := "foo"
	p1 := k8sclient.NewPlugin()
	p2 := k8sclient.NewPlugin()
	p3 := k8sclient.NewPlugin(k8sclient.UseDeps(&k8sclient.Deps{Name: name}))
	testsuites.SuiteRegistry(t, p1, p2, p3)
}
