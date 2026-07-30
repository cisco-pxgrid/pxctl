package cmd

import (
	"encoding/json"
	"fmt"
	"os"
)

type jsonObjectStream struct {
	file     *os.File
	decoder  *json.Decoder
	inArray  bool
	finished bool
}

func newJSONObjectStream(path string) (*jsonObjectStream, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	s := &jsonObjectStream{file: file, decoder: json.NewDecoder(file)}
	token, err := s.decoder.Token()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		file.Close()
		return nil, fmt.Errorf("input JSON must be a top-level object containing an array")
	}

	// Locate the first top-level array. This matches the existing input format,
	// while leaving all record objects streamed through Decoder.Decode.
	for s.decoder.More() {
		key, err := s.decoder.Token()
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("invalid top-level JSON object: %w", err)
		}
		if _, ok := key.(string); !ok {
			file.Close()
			return nil, fmt.Errorf("invalid top-level JSON object key")
		}
		value, err := s.decoder.Token()
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("invalid top-level JSON value: %w", err)
		}
		if delim, ok := value.(json.Delim); ok && delim == '[' {
			s.inArray = true
			return s, nil
		}
		if err := skipJSONValue(s.decoder, value); err != nil {
			file.Close()
			return nil, err
		}
	}

	file.Close()
	return nil, fmt.Errorf("input JSON contains no top-level array")
}

func (s *jsonObjectStream) Next() (map[string]interface{}, bool, error) {
	if s.finished {
		return nil, true, nil
	}
	if !s.decoder.More() {
		if _, err := s.decoder.Token(); err != nil {
			return nil, false, fmt.Errorf("invalid JSON array: %w", err)
		}
		s.finished = true
		return nil, true, nil
	}

	var object map[string]interface{}
	if err := s.decoder.Decode(&object); err != nil {
		return nil, false, fmt.Errorf("failed to decode JSON object: %w", err)
	}
	if object == nil {
		return nil, false, fmt.Errorf("JSON array contains a non-object value")
	}
	return object, false, nil
}

func (s *jsonObjectStream) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	return s.file.Close()
}

func countJSONObjects(path string) (int, error) {
	stream, err := newJSONObjectStream(path)
	if err != nil {
		return 0, err
	}
	defer stream.Close()

	count := 0
	for {
		_, done, err := stream.Next()
		if err != nil {
			return 0, err
		}
		if done {
			return count, nil
		}
		count++
	}
}

func skipJSONValue(decoder *json.Decoder, token json.Token) error {
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		for decoder.More() {
			if _, err := decoder.Token(); err != nil {
				return err
			}
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := skipJSONValue(decoder, value); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	case '[':
		for decoder.More() {
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := skipJSONValue(decoder, value); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	default:
		return nil
	}
}
