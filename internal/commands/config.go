package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"honch.dev/honch/internal/config"
	"honch.dev/honch/internal/ui"
)

type configFieldKind int

const (
	configFieldString configFieldKind = iota
	configFieldInt
)

type configField struct {
	Key       string
	Name      string
	TypeLabel string
	Kind      configFieldKind
	Path      []string
	Read      func(config.Config) any
}

type configSection struct {
	Name   string
	Fields []configField
}

var configSections = []configSection{
	{
		Name: "repos",
		Fields: []configField{
			{Key: "repos.capture", Name: "capture", TypeLabel: "path", Kind: configFieldString, Path: []string{"repos", "capture"}, Read: func(cfg config.Config) any { return cfg.Repos.Capture }},
			{Key: "repos.platform", Name: "platform", TypeLabel: "path", Kind: configFieldString, Path: []string{"repos", "platform"}, Read: func(cfg config.Config) any { return cfg.Repos.Platform }},
			{Key: "repos.worker", Name: "worker", TypeLabel: "path", Kind: configFieldString, Path: []string{"repos", "worker"}, Read: func(cfg config.Config) any { return cfg.Repos.Worker }},
		},
	},
	{
		Name: "repo sources",
		Fields: []configField{
			{Key: "repo_sources.capture", Name: "capture", TypeLabel: "url", Kind: configFieldString, Path: []string{"repo_sources", "capture"}, Read: func(cfg config.Config) any { return cfg.RepoSources.Capture }},
			{Key: "repo_sources.platform", Name: "platform", TypeLabel: "url", Kind: configFieldString, Path: []string{"repo_sources", "platform"}, Read: func(cfg config.Config) any { return cfg.RepoSources.Platform }},
			{Key: "repo_sources.worker", Name: "worker", TypeLabel: "url", Kind: configFieldString, Path: []string{"repo_sources", "worker"}, Read: func(cfg config.Config) any { return cfg.RepoSources.Worker }},
		},
	},
	{
		Name: "ports",
		Fields: []configField{
			{Key: "ports.capture", Name: "capture", TypeLabel: "int", Kind: configFieldInt, Path: []string{"ports", "capture"}, Read: func(cfg config.Config) any { return cfg.Ports.Capture }},
			{Key: "ports.worker", Name: "worker", TypeLabel: "int", Kind: configFieldInt, Path: []string{"ports", "worker"}, Read: func(cfg config.Config) any { return cfg.Ports.Worker }},
			{Key: "ports.clickhouse", Name: "clickhouse", TypeLabel: "int", Kind: configFieldInt, Path: []string{"ports", "clickhouse"}, Read: func(cfg config.Config) any { return cfg.Ports.ClickHouse }},
			{Key: "ports.proxy", Name: "proxy", TypeLabel: "int", Kind: configFieldInt, Path: []string{"ports", "proxy"}, Read: func(cfg config.Config) any { return cfg.Ports.Proxy }},
			{Key: "ports.control", Name: "control", TypeLabel: "int", Kind: configFieldInt, Path: []string{"ports", "control"}, Read: func(cfg config.Config) any { return cfg.Ports.Control }},
		},
	},
	{
		Name: "sandbox",
		Fields: []configField{
			{Key: "sandbox.project_id", Name: "project_id", TypeLabel: "string", Kind: configFieldString, Path: []string{"sandbox", "project_id"}, Read: func(cfg config.Config) any { return cfg.Sandbox.ProjectID }},
			{Key: "sandbox.token", Name: "token", TypeLabel: "string", Kind: configFieldString, Path: []string{"sandbox", "token"}, Read: func(cfg config.Config) any { return cfg.Sandbox.Token }},
			{Key: "sandbox.clickhouse_database", Name: "clickhouse_database", TypeLabel: "string", Kind: configFieldString, Path: []string{"sandbox", "clickhouse_database"}, Read: func(cfg config.Config) any { return cfg.Sandbox.ClickHouseDatabase }},
			{Key: "sandbox.state_dir", Name: "state_dir", TypeLabel: "path", Kind: configFieldString, Path: []string{"sandbox", "state_dir"}, Read: func(cfg config.Config) any { return cfg.Sandbox.StateDir }},
			{Key: "sandbox.endpoint_url", Name: "endpoint_url", TypeLabel: "url", Kind: configFieldString, Path: []string{"sandbox", "endpoint_url"}, Read: func(cfg config.Config) any { return cfg.Sandbox.EndpointURL }},
			{Key: "sandbox.proxy_bind", Name: "proxy_bind", TypeLabel: "host", Kind: configFieldString, Path: []string{"sandbox", "proxy_bind"}, Read: func(cfg config.Config) any { return cfg.Sandbox.ProxyBind }},
			{Key: "sandbox.service_host", Name: "service_host", TypeLabel: "host", Kind: configFieldString, Path: []string{"sandbox", "service_host"}, Read: func(cfg config.Config) any { return cfg.Sandbox.ServiceHost }},
			{Key: "sandbox.docker_host", Name: "docker_host", TypeLabel: "url", Kind: configFieldString, Path: []string{"sandbox", "docker_host"}, Read: func(cfg config.Config) any { return cfg.Sandbox.DockerHost }},
			{Key: "sandbox.idf_path", Name: "idf_path", TypeLabel: "path", Kind: configFieldString, Path: []string{"sandbox", "idf_path"}, Read: func(cfg config.Config) any { return cfg.Sandbox.IDFPath }},
		},
	},
}

var configFieldByKey = func() map[string]configField {
	fields := map[string]configField{}
	for _, section := range configSections {
		for _, field := range section.Fields {
			fields[field.Key] = field
		}
	}
	return fields
}()

func newConfigCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "List and edit sandbox config",
		Args:  rejectUnknownArgs,
		RunE:  commandGroupRunE,
	}
	cmd.AddCommand(
		newConfigListCommand(deps),
		newConfigSetCommand(deps),
		newConfigEditCommand(deps),
		newConfigInitCommand(deps),
	)
	return cmd
}

func newConfigListCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show the current sandbox config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), formatConfigList(cfg))
			return nil
		},
	}
}

func newConfigSetCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Update a sandbox config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			field, ok := configFieldByKey[args[0]]
			if !ok {
				return fmt.Errorf("unsupported config key %q; use honch sandbox config list", args[0])
			}
			if err := setConfigValue(root, cfg, field, args[1]); err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Honch sandbox config", []ui.Section{{
				Rows: []ui.Row{
					{Key: "updated", Value: args[0]},
					{Key: "file", Value: configFilePath(root)},
				},
			}}))
			return nil
		},
	}
}

func newConfigEditCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open the sandbox config in your editor",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			path, err := ensureConfigFile(root, cfg)
			if err != nil {
				return err
			}
			return openEditor(cmd.Context(), path)
		},
	}
}

func newConfigInitCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create a starter sandbox config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			path := configFilePath(root)
			if _, err := os.Stat(path); err == nil {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Honch sandbox config", []ui.Section{{
					Rows: []ui.Row{{Key: "status", Value: "already initialized"}, {Key: "file", Value: path}},
				}}))
				return nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := os.WriteFile(path, []byte(starterConfigContent(cfg)), 0o600); err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Honch sandbox config", []ui.Section{{
				Rows: []ui.Row{{Key: "created", Value: "starter config"}, {Key: "file", Value: path}},
			}}))
			return nil
		},
	}
}

func formatConfigList(cfg config.Config) string {
	var sections []ui.Section
	for _, section := range configSections {
		rows := make([]ui.Row, 0, len(section.Fields))
		for _, field := range section.Fields {
			rows = append(rows, ui.Row{
				Key:   field.Name + " (" + field.TypeLabel + ")",
				Value: displayConfigValue(field.Read(cfg)),
			})
		}
		sections = append(sections, ui.Section{Name: section.Name, Rows: rows})
	}

	var b strings.Builder
	b.WriteString(ui.FormatSectionsWrapped("Honch sandbox config", sections))
	b.WriteString("\n  Use 'honch sandbox config set <key> <value>' to update\n")
	b.WriteString("  Use 'honch sandbox config edit' to open the file\n")
	b.WriteString("  `sandbox.endpoint_url` follows `ports.capture` unless it is set explicitly\n")
	return b.String()
}

func displayConfigValue(value any) any {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return "<not set>"
		}
		return v
	default:
		return v
	}
}

func setConfigValue(root string, cfg config.Config, field configField, rawValue string) error {
	docPath := configFilePath(root)
	if field.Kind == configFieldInt {
		if _, err := strconv.Atoi(rawValue); err != nil {
			return fmt.Errorf("invalid integer for %s: %q", field.Key, rawValue)
		}
	}
	if _, err := ensureConfigFile(root, cfg); err != nil {
		return err
	}
	doc, err := loadYAMLDocument(docPath)
	if err != nil {
		return err
	}
	if err := updateYAMLValue(doc, field.Path, rawValue, field.Kind); err != nil {
		return err
	}
	return writeYAMLDocument(docPath, doc)
}

func configFilePath(root string) string {
	return filepath.Join(root, ".honch-sandbox.yaml")
}

func ensureConfigFile(root string, cfg config.Config) (string, error) {
	path := configFilePath(root)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.WriteFile(path, []byte(starterConfigContent(cfg)), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func starterConfigContent(cfg config.Config) string {
	type starterRepos struct {
		Capture  string `yaml:"capture"`
		Platform string `yaml:"platform"`
		Worker   string `yaml:"worker"`
	}
	type starterRepoSources struct {
		Capture  string `yaml:"capture"`
		Platform string `yaml:"platform"`
		Worker   string `yaml:"worker"`
	}
	type starterPorts struct {
		Capture    int `yaml:"capture"`
		Worker     int `yaml:"worker"`
		ClickHouse int `yaml:"clickhouse"`
		Proxy      int `yaml:"proxy"`
		Control    int `yaml:"control"`
	}
	type starterSandbox struct {
		ProjectID          string `yaml:"project_id"`
		Token              string `yaml:"token"`
		ClickHouseDatabase string `yaml:"clickhouse_database"`
		ProxyBind          string `yaml:"proxy_bind"`
		ServiceHost        string `yaml:"service_host"`
		DockerHost         string `yaml:"docker_host,omitempty"`
		StateDir           string `yaml:"state_dir"`
	}
	content, err := yaml.Marshal(struct {
		Repos       starterRepos       `yaml:"repos"`
		RepoSources starterRepoSources `yaml:"repo_sources"`
		Ports       starterPorts       `yaml:"ports"`
		Sandbox     starterSandbox     `yaml:"sandbox"`
	}{
		Repos: starterRepos{
			Capture:  cfg.Repos.Capture,
			Platform: cfg.Repos.Platform,
			Worker:   cfg.Repos.Worker,
		},
		RepoSources: starterRepoSources{
			Capture:  cfg.RepoSources.Capture,
			Platform: cfg.RepoSources.Platform,
			Worker:   cfg.RepoSources.Worker,
		},
		Ports: starterPorts{
			Capture:    cfg.Ports.Capture,
			Worker:     cfg.Ports.Worker,
			ClickHouse: cfg.Ports.ClickHouse,
			Proxy:      cfg.Ports.Proxy,
			Control:    cfg.Ports.Control,
		},
		Sandbox: starterSandbox{
			ProjectID:          cfg.Sandbox.ProjectID,
			Token:              cfg.Sandbox.Token,
			ClickHouseDatabase: cfg.Sandbox.ClickHouseDatabase,
			ProxyBind:          cfg.Sandbox.ProxyBind,
			ServiceHost:        cfg.Sandbox.ServiceHost,
			DockerHost:         cfg.Sandbox.DockerHost,
			StateDir:           cfg.Sandbox.StateDir,
		},
	})
	if err != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Honch sandbox overrides.\n")
	b.WriteString("# Edit the values below or use `honch sandbox config set <key> <value>`.\n")
	b.WriteString("# `sandbox.endpoint_url` is derived from `ports.capture` unless you set it explicitly.\n\n")
	b.Write(content)
	return b.String()
}

func loadYAMLDocument(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func writeYAMLDocument(path string, doc *yaml.Node) error {
	data, err := yamlMarshalDocument(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func yamlMarshalDocument(doc *yaml.Node) ([]byte, error) {
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(documentMapping(doc)); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func updateYAMLValue(doc *yaml.Node, path []string, rawValue string, kind configFieldKind) error {
	if len(path) == 0 {
		return errors.New("config path is empty")
	}
	root := documentMapping(doc)
	node := root
	for i, key := range path {
		if i == len(path)-1 {
			setMappingScalar(node, key, rawValue, kind)
			return nil
		}
		node = ensureMappingNode(node, key)
	}
	return nil
}

func documentMapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind != yaml.DocumentNode {
		doc.Kind = yaml.DocumentNode
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		root.Kind = yaml.MappingNode
		root.Tag = "!!map"
		root.Value = ""
		root.Style = 0
		root.Content = nil
	}
	return root
}

func ensureMappingNode(mapNode *yaml.Node, key string) *yaml.Node {
	if child := mappingValue(mapNode, key); child != nil {
		if child.Kind != yaml.MappingNode {
			child.Kind = yaml.MappingNode
			child.Tag = "!!map"
			child.Value = ""
			child.Style = 0
			child.Content = nil
		}
		return child
	}
	child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapNode.Content = append(mapNode.Content, scalarNode(key, "!!str"), child)
	return child
}

func mappingValue(mapNode *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(mapNode.Content)-1; i += 2 {
		if mapNode.Content[i].Value == key {
			return mapNode.Content[i+1]
		}
	}
	return nil
}

func setMappingScalar(mapNode *yaml.Node, key string, rawValue string, kind configFieldKind) {
	valueNode := scalarNode(rawValue, "!!str")
	if kind == configFieldInt {
		valueNode.Tag = "!!int"
	}
	for i := 0; i < len(mapNode.Content)-1; i += 2 {
		if mapNode.Content[i].Value == key {
			mapNode.Content[i+1] = valueNode
			return
		}
	}
	mapNode.Content = append(mapNode.Content, scalarNode(key, "!!str"), valueNode)
}

func scalarNode(value string, tag string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
}

func openEditor(ctx context.Context, path string) error {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", "$EDITOR \"$1\"", "honch-config-edit", path)
	cmd.Env = append(os.Environ(), "EDITOR="+editor)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
