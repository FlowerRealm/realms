package openai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func moveValue(jsonStr, fromPath, toPath string) (string, error) {
	sourceValue := gjson.Get(jsonStr, fromPath)
	if !sourceValue.Exists() {
		return jsonStr, fmt.Errorf("source path does not exist: %s", fromPath)
	}
	result, err := sjson.Set(jsonStr, toPath, sourceValue.Value())
	if err != nil {
		return "", err
	}
	return sjson.Delete(result, fromPath)
}

func copyValue(jsonStr, fromPath, toPath string) (string, error) {
	sourceValue := gjson.Get(jsonStr, fromPath)
	if !sourceValue.Exists() {
		return jsonStr, fmt.Errorf("source path does not exist: %s", fromPath)
	}
	return sjson.Set(jsonStr, toPath, sourceValue.Value())
}

func modifyValue(jsonStr, path string, value any, keepOrigin, isPrepend bool) (string, error) {
	current := gjson.Get(jsonStr, path)
	switch {
	case current.IsArray():
		return modifyArray(jsonStr, path, value, isPrepend)
	case current.Type == gjson.String:
		return modifyString(jsonStr, path, value, isPrepend)
	case current.Type == gjson.JSON:
		return mergeObjects(jsonStr, path, value, keepOrigin)
	}
	return jsonStr, fmt.Errorf("operation not supported for type: %v", current.Type)
}

func modifyArray(jsonStr, path string, value any, isPrepend bool) (string, error) {
	current := gjson.Get(jsonStr, path)
	var newArray []any

	addValue := func() {
		if arr, ok := value.([]any); ok {
			newArray = append(newArray, arr...)
		} else {
			newArray = append(newArray, value)
		}
	}
	addOriginal := func() {
		current.ForEach(func(_, val gjson.Result) bool {
			newArray = append(newArray, val.Value())
			return true
		})
	}

	if isPrepend {
		addValue()
		addOriginal()
	} else {
		addOriginal()
		addValue()
	}
	return sjson.Set(jsonStr, path, newArray)
}

func modifyString(jsonStr, path string, value any, isPrepend bool) (string, error) {
	current := gjson.Get(jsonStr, path)
	valueStr := fmt.Sprintf("%v", value)
	var newStr string
	if isPrepend {
		newStr = valueStr + current.String()
	} else {
		newStr = current.String() + valueStr
	}
	return sjson.Set(jsonStr, path, newStr)
}

func trimStringValue(jsonStr, path string, value any, isPrefix bool) (string, error) {
	current := gjson.Get(jsonStr, path)
	if current.Type != gjson.String {
		return jsonStr, fmt.Errorf("operation not supported for type: %v", current.Type)
	}
	if value == nil {
		return jsonStr, fmt.Errorf("trim value is required")
	}
	valueStr := fmt.Sprintf("%v", value)

	var newStr string
	if isPrefix {
		newStr = strings.TrimPrefix(current.String(), valueStr)
	} else {
		newStr = strings.TrimSuffix(current.String(), valueStr)
	}
	return sjson.Set(jsonStr, path, newStr)
}

func ensureStringAffix(jsonStr, path string, value any, isPrefix bool) (string, error) {
	current := gjson.Get(jsonStr, path)
	if current.Type != gjson.String {
		return jsonStr, fmt.Errorf("operation not supported for type: %v", current.Type)
	}
	if value == nil {
		return jsonStr, fmt.Errorf("ensure value is required")
	}
	valueStr := fmt.Sprintf("%v", value)
	if valueStr == "" {
		return jsonStr, fmt.Errorf("ensure value is required")
	}

	currentStr := current.String()
	if isPrefix {
		if strings.HasPrefix(currentStr, valueStr) {
			return jsonStr, nil
		}
		return sjson.Set(jsonStr, path, valueStr+currentStr)
	}
	if strings.HasSuffix(currentStr, valueStr) {
		return jsonStr, nil
	}
	return sjson.Set(jsonStr, path, currentStr+valueStr)
}

func transformStringValue(jsonStr, path string, transform func(string) string) (string, error) {
	current := gjson.Get(jsonStr, path)
	if current.Type != gjson.String {
		return jsonStr, fmt.Errorf("operation not supported for type: %v", current.Type)
	}
	return sjson.Set(jsonStr, path, transform(current.String()))
}

func replaceStringValue(jsonStr, path, from, to string) (string, error) {
	current := gjson.Get(jsonStr, path)
	if current.Type != gjson.String {
		return jsonStr, fmt.Errorf("operation not supported for type: %v", current.Type)
	}
	if from == "" {
		return jsonStr, fmt.Errorf("replace from is required")
	}
	return sjson.Set(jsonStr, path, strings.ReplaceAll(current.String(), from, to))
}

func regexReplaceStringValue(jsonStr, path, pattern, replacement string) (string, error) {
	current := gjson.Get(jsonStr, path)
	if current.Type != gjson.String {
		return jsonStr, fmt.Errorf("operation not supported for type: %v", current.Type)
	}
	if pattern == "" {
		return jsonStr, fmt.Errorf("regex pattern is required")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return jsonStr, err
	}
	return sjson.Set(jsonStr, path, re.ReplaceAllString(current.String(), replacement))
}

func mergeObjects(jsonStr, path string, value any, keepOrigin bool) (string, error) {
	current := gjson.Get(jsonStr, path)
	var currentMap map[string]any
	if err := json.Unmarshal([]byte(current.Raw), &currentMap); err != nil {
		return "", err
	}

	var newMap map[string]any
	switch v := value.(type) {
	case map[string]any:
		newMap = v
	default:
		jsonBytes, _ := json.Marshal(v)
		if err := json.Unmarshal(jsonBytes, &newMap); err != nil {
			return "", err
		}
	}

	result := make(map[string]any)
	for k, v := range currentMap {
		result[k] = v
	}
	for k, v := range newMap {
		if !keepOrigin || result[k] == nil {
			result[k] = v
		}
	}
	return sjson.Set(jsonStr, path, result)
}
