package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type outputFormat string

const (
	formatTable outputFormat = "table"
	formatJSON  outputFormat = "json"
	formatYAML  outputFormat = "yaml"
)

func parseOutputFormat(s string) (outputFormat, error) {
	switch strings.ToLower(s) {
	case "", "table":
		return formatTable, nil
	case "json":
		return formatJSON, nil
	case "yaml":
		return formatYAML, nil
	default:
		return "", fmt.Errorf("unsupported output format %q (use table, json, or yaml)", s)
	}
}

func printTable(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	for i, h := range headers {
		fmt.Printf("%-*s  ", widths[i], strings.ToUpper(h))
	}
	fmt.Println()

	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				fmt.Printf("%-*s  ", widths[i], cell)
			}
		}
		fmt.Println()
	}
}

func printStructured(format outputFormat, data any) error {
	switch format {
	case formatJSON:
		out, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
	case formatYAML:
		out, err := yaml.Marshal(data)
		if err != nil {
			return err
		}
		fmt.Print(string(out))
	}
	return nil
}
