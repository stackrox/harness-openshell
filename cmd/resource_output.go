package cmd

import "github.com/stackrox/harness-openshell/internal/openshell"

type sandboxOutput struct {
	Name  string `json:"name" yaml:"name"`
	Phase string `json:"phase" yaml:"phase"`
}

type providerOutput struct {
	Name string `json:"name" yaml:"name"`
}

type gatewayOutput struct {
	Name     string `json:"name" yaml:"name"`
	Endpoint string `json:"endpoint" yaml:"endpoint"`
	Status   string `json:"status" yaml:"status"`
	Version  string `json:"version" yaml:"version"`
}

type describeOutput struct {
	Name      string   `json:"name" yaml:"name"`
	Phase     string   `json:"phase" yaml:"phase"`
	Gateway   string   `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	Endpoint  string   `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Providers []string `json:"providers,omitempty" yaml:"providers,omitempty"`
}

func sandboxOutputs(sandboxes []openshell.Sandbox) []sandboxOutput {
	out := make([]sandboxOutput, len(sandboxes))
	for i, sandbox := range sandboxes {
		out[i] = sandboxOutput{Name: sandbox.Name, Phase: sandbox.Phase}
	}
	return out
}

func providerOutputs(providers []openshell.Provider) []providerOutput {
	out := make([]providerOutput, len(providers))
	for i, provider := range providers {
		out[i] = providerOutput{Name: provider.Name}
	}
	return out
}

func providerNames(providers []openshell.Provider) []string {
	out := make([]string, len(providers))
	for i, provider := range providers {
		out[i] = provider.Name
	}
	return out
}

func gatewayRecord(info openshell.GatewayInfo) gatewayOutput {
	return gatewayOutput{
		Name:     info.Name,
		Endpoint: info.Endpoint,
		Status:   info.Status,
		Version:  info.Version,
	}
}

func describeRecord(sandbox openshell.Sandbox, info openshell.GatewayInfo, providers []openshell.Provider) describeOutput {
	return describeOutput{
		Name:      sandbox.Name,
		Phase:     sandbox.Phase,
		Gateway:   info.Name,
		Endpoint:  info.Endpoint,
		Providers: providerNames(providers),
	}
}
