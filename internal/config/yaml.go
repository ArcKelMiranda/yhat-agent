package config

import "gopkg.in/yaml.v3"

func yamlMarshal(v any) ([]byte, error) {
	return yaml.Marshal(v)
}

func yamlUnmarshal(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}
