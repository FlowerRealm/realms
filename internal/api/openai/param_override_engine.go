package openai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var negativeIndexRegexp = regexp.MustCompile(`\\.(-\\d+)`)

type ConditionOperation struct {
	Path           string `json:"path"`
	Mode           string `json:"mode"` // full, prefix, suffix, contains, gt, gte, lt, lte
	Value          any    `json:"value"`
	Invert         bool   `json:"invert"`
	PassMissingKey bool   `json:"pass_missing_key"`
}

type ParamOperation struct {
	Path       string               `json:"path"`
	Mode       string               `json:"mode"` // delete, set, move, copy, prepend, append, trim_prefix, trim_suffix, ensure_prefix, ensure_suffix, trim_space, to_lower, to_upper, replace, regex_replace
	Value      any                  `json:"value"`
	KeepOrigin bool                 `json:"keep_origin"`
	From       string               `json:"from,omitempty"`
	To         string               `json:"to,omitempty"`
	Conditions []ConditionOperation `json:"conditions,omitempty"`
	Logic      string               `json:"logic,omitempty"` // AND, OR（默认 OR）
}

func ApplyParamOverride(jsonData []byte, paramOverride map[string]any, conditionContext map[string]any) ([]byte, error) {
	if len(paramOverride) == 0 {
		return jsonData, nil
	}

	if operations, ok := tryParseOperations(paramOverride); ok {
		result, err := applyOperations(string(jsonData), operations, conditionContext)
		return []byte(result), err
	}

	return applyOperationsLegacy(jsonData, paramOverride)
}

func tryParseOperations(paramOverride map[string]any) ([]ParamOperation, bool) {
	opsValue, exists := paramOverride["operations"]
	if !exists {
		return nil, false
	}
	opsSlice, ok := opsValue.([]any)
	if !ok {
		return nil, false
	}

	var operations []ParamOperation
	for _, op := range opsSlice {
		opMap, ok := op.(map[string]any)
		if !ok {
			return nil, false
		}
		operation := ParamOperation{}

		if path, ok := opMap["path"].(string); ok {
			operation.Path = path
		}
		if mode, ok := opMap["mode"].(string); ok {
			operation.Mode = mode
		} else {
			return nil, false
		}

		if value, exists := opMap["value"]; exists {
			operation.Value = value
		}
		if keepOrigin, ok := opMap["keep_origin"].(bool); ok {
			operation.KeepOrigin = keepOrigin
		}
		if from, ok := opMap["from"].(string); ok {
			operation.From = from
		}
		if to, ok := opMap["to"].(string); ok {
			operation.To = to
		}
		if logic, ok := opMap["logic"].(string); ok {
			operation.Logic = logic
		} else {
			operation.Logic = "OR"
		}

		if conditions, exists := opMap["conditions"]; exists {
			if condSlice, ok := conditions.([]any); ok {
				for _, cond := range condSlice {
					if condMap, ok := cond.(map[string]any); ok {
						condition := ConditionOperation{}
						if path, ok := condMap["path"].(string); ok {
							condition.Path = path
						}
						if mode, ok := condMap["mode"].(string); ok {
							condition.Mode = mode
						}
						if value, ok := condMap["value"]; ok {
							condition.Value = value
						}
						if invert, ok := condMap["invert"].(bool); ok {
							condition.Invert = invert
						}
						if passMissingKey, ok := condMap["pass_missing_key"].(bool); ok {
							condition.PassMissingKey = passMissingKey
						}
						operation.Conditions = append(operation.Conditions, condition)
					}
				}
			}
		}

		operations = append(operations, operation)
	}
	return operations, true
}

func checkConditions(jsonStr, contextJSON string, conditions []ConditionOperation, logic string) (bool, error) {
	if len(conditions) == 0 {
		return true, nil
	}
	results := make([]bool, len(conditions))
	for i, condition := range conditions {
		result, err := checkSingleCondition(jsonStr, contextJSON, condition)
		if err != nil {
			return false, err
		}
		results[i] = result
	}

	if strings.ToUpper(logic) == "AND" {
		for _, result := range results {
			if !result {
				return false, nil
			}
		}
		return true, nil
	}
	for _, result := range results {
		if result {
			return true, nil
		}
	}
	return false, nil
}

func checkSingleCondition(jsonStr, contextJSON string, condition ConditionOperation) (bool, error) {
	path := processNegativeIndex(jsonStr, condition.Path)
	value := gjson.Get(jsonStr, path)
	if !value.Exists() && contextJSON != "" {
		value = gjson.Get(contextJSON, condition.Path)
	}
	if !value.Exists() {
		if condition.PassMissingKey {
			return true, nil
		}
		return false, nil
	}

	targetBytes, err := json.Marshal(condition.Value)
	if err != nil {
		return false, fmt.Errorf("marshal condition value failed: %v", err)
	}
	targetValue := gjson.ParseBytes(targetBytes)

	result, err := compareGjsonValues(value, targetValue, strings.ToLower(condition.Mode))
	if err != nil {
		return false, fmt.Errorf("comparison failed for path %s: %v", condition.Path, err)
	}
	if condition.Invert {
		result = !result
	}
	return result, nil
}

func processNegativeIndex(jsonStr string, path string) string {
	matches := negativeIndexRegexp.FindAllStringSubmatch(path, -1)
	if len(matches) == 0 {
		return path
	}

	result := path
	for _, match := range matches {
		negIndex := match[1]
		index, _ := strconv.Atoi(negIndex)

		arrayPath := strings.Split(path, negIndex)[0]
		arrayPath = strings.TrimSuffix(arrayPath, ".")

		array := gjson.Get(jsonStr, arrayPath)
		if array.IsArray() {
			length := len(array.Array())
			actualIndex := length + index
			if actualIndex >= 0 && actualIndex < length {
				result = strings.Replace(result, match[0], "."+strconv.Itoa(actualIndex), 1)
			}
		}
	}
	return result
}

func compareGjsonValues(jsonValue, targetValue gjson.Result, mode string) (bool, error) {
	switch mode {
	case "full":
		return compareEqual(jsonValue, targetValue)
	case "prefix":
		return strings.HasPrefix(jsonValue.String(), targetValue.String()), nil
	case "suffix":
		return strings.HasSuffix(jsonValue.String(), targetValue.String()), nil
	case "contains":
		return strings.Contains(jsonValue.String(), targetValue.String()), nil
	case "gt":
		return compareNumeric(jsonValue, targetValue, "gt")
	case "gte":
		return compareNumeric(jsonValue, targetValue, "gte")
	case "lt":
		return compareNumeric(jsonValue, targetValue, "lt")
	case "lte":
		return compareNumeric(jsonValue, targetValue, "lte")
	default:
		return false, fmt.Errorf("unsupported comparison mode: %s", mode)
	}
}

func compareEqual(jsonValue, targetValue gjson.Result) (bool, error) {
	if jsonValue.Type == gjson.Null || targetValue.Type == gjson.Null {
		return jsonValue.Type == gjson.Null && targetValue.Type == gjson.Null, nil
	}

	if (jsonValue.Type == gjson.True || jsonValue.Type == gjson.False) &&
		(targetValue.Type == gjson.True || targetValue.Type == gjson.False) {
		return jsonValue.Bool() == targetValue.Bool(), nil
	}

	if jsonValue.Type != targetValue.Type {
		return false, fmt.Errorf("compare for different types, got %v and %v", jsonValue.Type, targetValue.Type)
	}

	switch jsonValue.Type {
	case gjson.Number:
		return jsonValue.Num == targetValue.Num, nil
	case gjson.String:
		return jsonValue.String() == targetValue.String(), nil
	default:
		return jsonValue.String() == targetValue.String(), nil
	}
}

func compareNumeric(jsonValue, targetValue gjson.Result, operator string) (bool, error) {
	if jsonValue.Type != gjson.Number || targetValue.Type != gjson.Number {
		return false, fmt.Errorf("numeric comparison requires both values to be numbers, got %v and %v", jsonValue.Type, targetValue.Type)
	}
	jsonNum := jsonValue.Num
	targetNum := targetValue.Num
	switch operator {
	case "gt":
		return jsonNum > targetNum, nil
	case "gte":
		return jsonNum >= targetNum, nil
	case "lt":
		return jsonNum < targetNum, nil
	case "lte":
		return jsonNum <= targetNum, nil
	default:
		return false, fmt.Errorf("unsupported numeric operator: %s", operator)
	}
}

func applyOperationsLegacy(jsonData []byte, paramOverride map[string]any) ([]byte, error) {
	reqMap := make(map[string]any)
	if err := json.Unmarshal(jsonData, &reqMap); err != nil {
		return nil, err
	}
	for key, value := range paramOverride {
		reqMap[key] = value
	}
	return json.Marshal(reqMap)
}

func applyOperations(jsonStr string, operations []ParamOperation, conditionContext map[string]any) (string, error) {
	var contextJSON string
	if len(conditionContext) > 0 {
		ctxBytes, err := json.Marshal(conditionContext)
		if err != nil {
			return "", fmt.Errorf("marshal condition context failed: %v", err)
		}
		contextJSON = string(ctxBytes)
	}

	result := jsonStr
	for _, op := range operations {
		ok, err := checkConditions(result, contextJSON, op.Conditions, op.Logic)
		if err != nil {
			return "", err
		}
		if !ok {
			continue
		}

		opPath := processNegativeIndex(result, op.Path)

		switch op.Mode {
		case "delete":
			result, err = sjson.Delete(result, opPath)
		case "set":
			if op.KeepOrigin && gjson.Get(result, opPath).Exists() {
				continue
			}
			result, err = sjson.Set(result, opPath, op.Value)
		case "move":
			opFrom := processNegativeIndex(result, op.From)
			opTo := processNegativeIndex(result, op.To)
			result, err = moveValue(result, opFrom, opTo)
		case "copy":
			if op.From == "" || op.To == "" {
				return "", fmt.Errorf("copy from/to is required")
			}
			opFrom := processNegativeIndex(result, op.From)
			opTo := processNegativeIndex(result, op.To)
			result, err = copyValue(result, opFrom, opTo)
		case "prepend":
			result, err = modifyValue(result, opPath, op.Value, op.KeepOrigin, true)
		case "append":
			result, err = modifyValue(result, opPath, op.Value, op.KeepOrigin, false)
		case "trim_prefix":
			result, err = trimStringValue(result, opPath, op.Value, true)
		case "trim_suffix":
			result, err = trimStringValue(result, opPath, op.Value, false)
		case "ensure_prefix":
			result, err = ensureStringAffix(result, opPath, op.Value, true)
		case "ensure_suffix":
			result, err = ensureStringAffix(result, opPath, op.Value, false)
		case "trim_space":
			result, err = transformStringValue(result, opPath, strings.TrimSpace)
		case "to_lower":
			result, err = transformStringValue(result, opPath, strings.ToLower)
		case "to_upper":
			result, err = transformStringValue(result, opPath, strings.ToUpper)
		case "replace":
			result, err = replaceStringValue(result, opPath, op.From, op.To)
		case "regex_replace":
			result, err = regexReplaceStringValue(result, opPath, op.From, op.To)
		default:
			return "", fmt.Errorf("unknown operation: %s", op.Mode)
		}
		if err != nil {
			return "", fmt.Errorf("operation %s failed: %v", op.Mode, err)
		}
	}
	return result, nil
}
