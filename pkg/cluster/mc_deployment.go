// Copyright 2019 Tad Lebeck
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


package cluster

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"sync"
	TT "text/template"

	"github.com/Masterminds/semver"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/op/go-logging"
)

// MCDeployer provides the managed cluster deployment support
type MCDeployer interface {
	GetMCDeployment(args *MCDeploymentArgs) (*Deployment, error)
}

// Deployment returns text containing deployment configuration
// e.g. Deployment: <kubectl yaml doc>, Format: "yaml"
type Deployment struct {
	Deployment string
	Format     string
}

// default values for managed cluster deployment
const (
	MCDeployDefaultArch           = "amd64"
	MCDeployDefaultClusterType    = K8sClusterType
	MCDeployDefaultNuvoPort       = 32145
	MCDeployDefaultOS             = "linux"
	MCDeployDefaultDriverType     = "csi"
	MCDeployDefaultClusterVersion = K8sDefaultSupportedVersion

	DefaultMCTemplateDir = "/opt/nuvoloso/lib/deploy"
)

// MCDeploySupportedClusterTypes describes the supporeted cluster orchestrator types
var MCDeploySupportedClusterTypes = []string{K8sClusterType}

// MCDeploymentArgs contain the properties required to create the managed cluster deployment document
type MCDeploymentArgs struct {
	AgentdCert      string // base64 encoding required
	AgentdKey       string // base64 encoding required
	Arch            string // default: amd64
	CACert          string // base64 encoding required
	CSPDomainID     string
	CSPDomainType   string // default: from CSP
	ClusterID       string
	ClusterName     string // default: derived from cluster-identifier by clusterd
	ClusterType     string // default: MCDeployDefaultClusterType
	ClusterVersion  string // default: MCDeployDefaultClusterVersion
	ClusterdCert    string // base64 encoding required
	ClusterdKey     string // base64 encoding required
	DriverType      string // default: csi
	ImagePath       string
	ImagePullSecret string // optional base64 encoding required
	ImageTag        string // default: latest
	ManagementHost  string
	NuvoPort        int    // default: 32145
	OS              string // default: linux
	SystemID        string
}

// SetDefaults initializes the type with default values
func (args *MCDeploymentArgs) SetDefaults() {
	if args.Arch == "" {
		args.Arch = MCDeployDefaultArch
	}
	if args.OS == "" {
		args.OS = MCDeployDefaultOS
	}
	if args.ClusterType == "" {
		args.ClusterType = MCDeployDefaultClusterType
	}
	if args.ClusterVersion == "" {
		if args.ClusterType == MCDeployDefaultClusterType {
			args.ClusterVersion = MCDeployDefaultClusterVersion
		}
	}
	if args.NuvoPort <= 0 || args.NuvoPort > 65535 {
		args.NuvoPort = MCDeployDefaultNuvoPort
	}
	if args.DriverType == "" {
		args.DriverType = MCDeployDefaultDriverType
	}
}

// Validate checks that the arguments are filled in after defaults filled in
func (args *MCDeploymentArgs) Validate() error {
	args.SetDefaults()
	invalid := []string{}
	if !args.isBase64(args.AgentdCert) {
		invalid = append(invalid, "AgentdCert")
	}
	if !args.isBase64(args.AgentdKey) {
		invalid = append(invalid, "AgentdKey")
	}
	if !args.isBase64(args.CACert) {
		invalid = append(invalid, "CACert")
	}
	if !args.isBase64(args.ClusterdCert) {
		invalid = append(invalid, "ClusterdCert")
	}
	if !args.isBase64(args.ClusterdKey) {
		invalid = append(invalid, "ClusterdKey")
	}
	if args.ClusterID == "" {
		invalid = append(invalid, "ClusterID")
	}
	if args.ImagePath == "" {
		invalid = append(invalid, "ImagePath")
	}
	if args.ImageTag == "" {
		invalid = append(invalid, "ImageTag")
	}
	if args.SystemID == "" {
		invalid = append(invalid, "SystemID")
	}
	if args.CSPDomainID == "" {
		invalid = append(invalid, "CSPDomainID")
	}
	if args.CSPDomainType == "" {
		invalid = append(invalid, "CSPDomainType")
	}
	if args.ManagementHost == "" {
		invalid = append(invalid, "ManagementHost")
	}
	if args.ImagePullSecret != "" && !args.isBase64(args.ImagePullSecret) {
		invalid = append(invalid, "ImagePullSecret")
	}
	if !util.Contains(MCDeploySupportedClusterTypes, args.ClusterType) {
		invalid = append(invalid, "ClusterType")
	}
	if args.ClusterType == K8sClusterType {
		c, _ := semver.NewConstraint(K8sSupportedVersionsConstraint)
		if semVer, err := semver.NewVersion(args.ClusterVersion); err != nil || !c.Check(semVer) {
			invalid = append(invalid, "ClusterVersion")
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("invalid arguments: %v", invalid)
	}
	return nil
}

func (args *MCDeploymentArgs) isBase64(val string) bool {
	if val != "" {
		if _, err := base64.StdEncoding.DecodeString(val); err == nil {
			return true
		}
	}
	return false
}

var mcMutex sync.Mutex

var defaultMCTemplateProcessor MCDeployer // singleton

// GetMCDeployer returns an MCDeployer
func GetMCDeployer(log *logging.Logger) MCDeployer {
	if defaultMCTemplateProcessor == nil {
		mcMutex.Lock()
		defer mcMutex.Unlock()
		if defaultMCTemplateProcessor == nil {
			defaultMCTemplateProcessor = &mcTemplateProcessor{log: log}
		}
	}
	return defaultMCTemplateProcessor
}

var mcTemplateDir = DefaultMCTemplateDir

type mcTemplateProcessor struct {
	log *logging.Logger
	tt  *TT.Template
}

func (tf *mcTemplateProcessor) validateSemverStrings(sem1 string, sem2 string, comp string) (bool, error) {
	sem1Ver, err := semver.NewVersion(sem1)
	if err != nil {
		return false, err
	}
	sem2Ver, err := semver.NewVersion(sem2)
	if err != nil {
		return false, err
	}
	rc := false
	switch comp {
	case "<":
		rc = sem1Ver.LessThan(sem2Ver)
	case "<=":
		rc = sem1Ver.LessThan(sem2Ver) || sem1Ver.Equal(sem2Ver)
	case "=":
		rc = sem1Ver.Equal(sem2Ver)
	case ">=":
		rc = sem1Ver.GreaterThan(sem2Ver) || sem1Ver.Equal(sem2Ver)
	case ">":
		rc = sem1Ver.GreaterThan(sem2Ver)
	default:
		return false, fmt.Errorf("invalid semver comparison")
	}
	return rc, nil
}

func (tf *mcTemplateProcessor) semverLessThan(sem1 string, sem2 string) (bool, error) {
	return tf.validateSemverStrings(sem1, sem2, "<")
}

func (tf *mcTemplateProcessor) semverLessThanEquals(sem1 string, sem2 string) (bool, error) {
	return tf.validateSemverStrings(sem1, sem2, "<=")
}

func (tf *mcTemplateProcessor) semverEquals(sem1 string, sem2 string) (bool, error) {
	return tf.validateSemverStrings(sem1, sem2, "=")
}

func (tf *mcTemplateProcessor) semverGreaterThanEquals(sem1 string, sem2 string) (bool, error) {
	return tf.validateSemverStrings(sem1, sem2, ">=")
}

func (tf *mcTemplateProcessor) semverGreaterThan(sem1 string, sem2 string) (bool, error) {
	return tf.validateSemverStrings(sem1, sem2, ">")
}

// lazy initialization allows factory method to be used
// in other module UTs where runtime path is missing
func (tf *mcTemplateProcessor) init() error {
	if tf.tt == nil {
		mcMutex.Lock()
		defer mcMutex.Unlock()
		if tf.tt == nil {
			tt, err := TT.New(mcTemplateDir + "/mc*.yaml.tmpl").Funcs(TT.FuncMap{
				"SemVerLT": tf.semverLessThan,
				"SemVerLE": tf.semverLessThanEquals,
				"SemVerEQ": tf.semverEquals,
				"SemVerGE": tf.semverGreaterThanEquals,
				"SemVerGT": tf.semverGreaterThan,
			}).ParseGlob(mcTemplateDir + "/mc*.yaml.tmpl")
			if err != nil {
				return err
			}
			tf.tt = tt
		}
	}
	return nil
}

// findTemplate returns a template that satisfies the selection criteria of the arguments
func (tf *mcTemplateProcessor) findTemplate(args *MCDeploymentArgs) *TT.Template {
	names := []string{
		"mc-" + args.CSPDomainType + args.DriverType + ".yaml.tmpl",
		"mc-" + args.CSPDomainType + ".yaml.tmpl",
		"mc-" + args.DriverType + ".yaml.tmpl",
		"mc.yaml.tmpl",
	}
	for _, n := range names {
		if t := tf.tt.Lookup(n); t != nil {
			tf.log.Debug("Found template", n)
			return t
		}
		tf.log.Debug("Did not find template", n)
	}
	return nil
}

// GetMCDeployment satisfies the MCDeployer interface
// It finds and executes a template
func (tf *mcTemplateProcessor) GetMCDeployment(args *MCDeploymentArgs) (*Deployment, error) {
	var err error
	if err = tf.init(); err == nil {
		err = args.Validate()
	}
	if err != nil {
		return nil, err
	}
	tmpl := tf.findTemplate(args)
	if tmpl == nil {
		return nil, fmt.Errorf("could not find appropriate deployment template for %s", args.CSPDomainType)
	}
	b := &bytes.Buffer{}
	if err := tmpl.Execute(b, args); err != nil {
		return nil, err
	}
	return &Deployment{
		Format:     "yaml",
		Deployment: b.String(),
	}, nil
}
