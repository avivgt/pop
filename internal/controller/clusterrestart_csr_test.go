/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"testing"

	certificatesv1 "k8s.io/api/certificates/v1"
)

// makeCSRRequest builds a real PEM-encoded certificate request with the given
// subject CN and organizations, signed by a fresh key.
func makeCSRRequest(t *testing.T, cn string, orgs []string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: cn, Organization: orgs},
	}, key)
	if err != nil {
		t.Fatalf("create CSR: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

const (
	clientSigner  = "kubernetes.io/kube-apiserver-client-kubelet"
	servingSigner = "kubernetes.io/kubelet-serving"
	bootstrapper  = "system:serviceaccount:openshift-machine-config-operator:node-bootstrapper"
)

var (
	clientUsages  = []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageKeyEncipherment, certificatesv1.UsageClientAuth}
	servingUsages = []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageKeyEncipherment, certificatesv1.UsageServerAuth}
)

func TestValidateKubeletCSR(t *testing.T) {
	cases := []struct {
		name     string
		request  []byte
		signer   string
		usages   []certificatesv1.KeyUsage
		username string
		wantOK   bool
		wantNode string
	}{
		{
			name:     "valid client CSR from bootstrapper",
			request:  makeCSRRequest(t, "system:node:worker-1", []string{"system:nodes"}),
			signer:   clientSigner,
			usages:   clientUsages,
			username: bootstrapper,
			wantOK:   true,
			wantNode: "worker-1",
		},
		{
			name:     "valid client CSR renewal from node",
			request:  makeCSRRequest(t, "system:node:worker-1", []string{"system:nodes"}),
			signer:   clientSigner,
			usages:   clientUsages,
			username: "system:node:worker-1",
			wantOK:   true,
			wantNode: "worker-1",
		},
		{
			name:     "valid serving CSR from node",
			request:  makeCSRRequest(t, "system:node:worker-1", []string{"system:nodes"}),
			signer:   servingSigner,
			usages:   servingUsages,
			username: "system:node:worker-1",
			wantOK:   true,
			wantNode: "worker-1",
		},
		{
			name:     "reject CN not system:node",
			request:  makeCSRRequest(t, "attacker", []string{"system:nodes"}),
			signer:   clientSigner,
			usages:   clientUsages,
			username: bootstrapper,
			wantOK:   false,
		},
		{
			name:     "reject not in system:nodes group",
			request:  makeCSRRequest(t, "system:node:worker-1", []string{"system:masters"}),
			signer:   clientSigner,
			usages:   clientUsages,
			username: bootstrapper,
			wantOK:   false,
		},
		{
			name:     "reject client CSR from arbitrary user",
			request:  makeCSRRequest(t, "system:node:worker-1", []string{"system:nodes"}),
			signer:   clientSigner,
			usages:   clientUsages,
			username: "developer",
			wantOK:   false,
		},
		{
			name:     "reject serving CSR from a different node identity",
			request:  makeCSRRequest(t, "system:node:worker-1", []string{"system:nodes"}),
			signer:   servingSigner,
			usages:   servingUsages,
			username: "system:node:worker-2",
			wantOK:   false,
		},
		{
			name:     "reject serving CSR requesting client auth",
			request:  makeCSRRequest(t, "system:node:worker-1", []string{"system:nodes"}),
			signer:   servingSigner,
			usages:   clientUsages,
			username: "system:node:worker-1",
			wantOK:   false,
		},
		{
			name:     "reject usages including cert sign",
			request:  makeCSRRequest(t, "system:node:worker-1", []string{"system:nodes"}),
			signer:   clientSigner,
			usages:   []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth, certificatesv1.UsageCertSign},
			username: bootstrapper,
			wantOK:   false,
		},
		{
			name:     "reject non-kubelet signer",
			request:  makeCSRRequest(t, "system:node:worker-1", []string{"system:nodes"}),
			signer:   "kubernetes.io/legacy-unknown",
			usages:   clientUsages,
			username: bootstrapper,
			wantOK:   false,
		},
		{
			name:     "reject garbage request bytes",
			request:  []byte("not a pem csr"),
			signer:   clientSigner,
			usages:   clientUsages,
			username: bootstrapper,
			wantOK:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			csr := &certificatesv1.CertificateSigningRequest{
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request:    tc.request,
					SignerName: tc.signer,
					Usages:     tc.usages,
					Username:   tc.username,
				},
			}
			node, ok, reason := validateKubeletCSR(csr)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v (reason %q), want %v", ok, reason, tc.wantOK)
			}
			if tc.wantOK && node != tc.wantNode {
				t.Fatalf("node = %q, want %q", node, tc.wantNode)
			}
			if !tc.wantOK && reason == "" {
				t.Errorf("expected a non-empty rejection reason")
			}
		})
	}
}
