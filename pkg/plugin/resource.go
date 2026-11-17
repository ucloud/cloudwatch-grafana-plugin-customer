package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// Option 统一返回给前端的下拉选项结构
type Option struct {
	Text  string `json:"text"`
	Value string `json:"value"`
	// GlobalService 仅产品列表使用：1 表示全局产品（前端隐藏地域选择器）
	GlobalService int `json:"globalService,omitempty"`
}

// CallResource 将前端 /projects、/regions、/products、/metrics、/resources、/aggregations
// 路由到对应的 UCloud OpenAPI
func (d *Datasource) CallResource(_ context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	query := parseQuery(req.URL)

	path := req.Path
	if path == "" {
		// 某些 Grafana 版本 req.Path 为空，需要自己从 URL 截取
		path = splitPath(req.URL)
	}
	// Grafana 传入的 req.Path 不带前导斜杠（如 "projects"），统一补齐后再路由
	path = normalizeResourcePath(path)

	var (
		options []Option
		err     error
	)

	switch path {
	case "/projects":
		options, err = d.listProjects()
	case "/products":
		options, err = d.listProducts()
	case "/regions":
		options, err = d.listRegions(query.Get("productKey"))
	case "/metrics":
		options, err = d.listMetrics(query.Get("productKey"))
	case "/resources":
		options, err = d.listResources(query.Get("productKey"), query.Get("region"), query.Get("projectId"))
	case "/aggregations":
		options, err = d.listAggregations()
	default:
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusNotFound,
			Body:   []byte(fmt.Sprintf(`{"error":"unknown path: %s"}`, path)),
		})
	}

	if err != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusBadGateway,
			Body:   []byte(fmt.Sprintf(`{"error":%q}`, err.Error())),
		})
	}

	body, _ := json.Marshal(options)
	return sender.Send(&backend.CallResourceResponse{
		Status:  http.StatusOK,
		Headers: map[string][]string{"Content-Type": {"application/json"}},
		Body:    body,
	})
}

// --- 具体资源拉取 ---

func (d *Datasource) listProjects() ([]Option, error) {
	resp, _, err := d.uclient.Invoke(map[string]interface{}{
		"Action": "GetProjectList",
	})
	if err != nil {
		return nil, err
	}
	if err := checkRetCode(resp, "GetProjectList"); err != nil {
		return nil, err
	}
	list := extractList(resp, "ProjectSet")
	log.DefaultLogger.Info("GetProjectList 解析结果",
		"listLen", len(list),
		"dataKeys", mapKeys(resp["Data"]),
	)
	out := make([]Option, 0, len(list))
	for _, it := range list {
		id := asString(it["ProjectId"])
		name := asString(it["ProjectName"])
		if name == "" {
			name = id
		}
		out = append(out, Option{Text: name, Value: id})
	}
	return out, nil
}

// 产品类型：ListMonitorProduct（无请求参数；响应 Data 直接是数组）
func (d *Datasource) listProducts() ([]Option, error) {
	resp, _, err := d.uclient.Invoke(map[string]interface{}{
		"Action": "ListMonitorProduct",
	})
	if err != nil {
		return nil, err
	}
	if err := checkRetCode(resp, "ListMonitorProduct"); err != nil {
		return nil, err
	}
	list := extractList(resp, "List")
	log.DefaultLogger.Info("ListMonitorProduct 解析结果", "listLen", len(list))
	out := make([]Option, 0, len(list))
	for _, it := range list {
		key := asString(it["ProductKey"])
		// 文档字段名两种写法都见过：ProductChName/ProductEnName 或 ProductCNName/ProductENName
		cn := firstNonEmpty(asString(it["ProductChName"]), asString(it["ProductCNName"]))
		en := firstNonEmpty(asString(it["ProductEnName"]), asString(it["ProductENName"]))
		// 展示格式：中文_英文
		var label string
		switch {
		case cn != "" && en != "":
			label = cn + "_" + en
		case cn != "":
			label = cn
		case en != "":
			label = en
		default:
			label = firstNonEmpty(asString(it["ProductName"]), key)
		}
		// 全局产品标记：值为 1 时前端隐藏地域选择器
		gs := 0
		if v, ok := it["GlobalService"].(float64); ok {
			gs = int(v)
		}
		out = append(out, Option{Text: label, Value: key, GlobalService: gs})
	}
	return out, nil
}

// 地域：ListRegions（无请求参数）
func (d *Datasource) listRegions(_ string) ([]Option, error) {
	resp, _, err := d.uclient.Invoke(map[string]interface{}{
		"Action": "ListRegions",
	})
	if err != nil {
		return nil, err
	}
	if err := checkRetCode(resp, "ListRegions"); err != nil {
		return nil, err
	}
	list := extractList(resp, "Regions")
	log.DefaultLogger.Info("ListRegions 解析结果", "listLen", len(list), "dataKeys", mapKeys(resp["Data"]))
	out := make([]Option, 0, len(list))
	seen := map[string]bool{}
	for _, it := range list {
		// 文档中具体字段名待确认，先做兼容：常见为 Region/RegionName 或 RegionCode/RegionName
		code := asString(it["Region"])
		if code == "" {
			code = asString(it["RegionCode"])
		}
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		name := asString(it["RegionName"])
		// PRD 格式：地域名称-地域code（例如 "墨西哥（墨西哥城）-mx-mex"）
		label := code
		if name != "" {
			label = fmt.Sprintf("%s-%s", name, code)
		}
		out = append(out, Option{Text: label, Value: code})
	}
	return out, nil
}

// 指标：GetProductMetrics（必填 ProductKey；响应 Data.List）
func (d *Datasource) listMetrics(productKey string) ([]Option, error) {
	if productKey == "" {
		return []Option{}, nil
	}
	resp, _, err := d.uclient.Invoke(map[string]interface{}{
		"Action":     "GetProductMetrics",
		"ProductKey": productKey,
	})
	if err != nil {
		return nil, err
	}
	if err := checkRetCode(resp, "GetProductMetrics"); err != nil {
		return nil, err
	}
	list := extractList(resp, "List")
	out := make([]Option, 0, len(list))
	for _, it := range list {
		metric := asString(it["Metric"])
		cn := firstNonEmpty(asString(it["MetricChName"]), asString(it["MetricCNName"]))
		en := firstNonEmpty(asString(it["MetricEnName"]), asString(it["MetricENName"]))
		// 展示格式：中文_英文
		var label string
		switch {
		case cn != "" && en != "":
			label = cn + "_" + en
		case cn != "":
			label = cn
		case en != "":
			label = en
		default:
			label = metric
		}
		out = append(out, Option{Text: label, Value: metric})
	}
	return out, nil
}

// 资源：ListResource（参数 Region/ProjectId）；返回格式待日志确认
func (d *Datasource) listResources(productKey, region, projectID string) ([]Option, error) {
	payload := map[string]interface{}{
		"Action": "ListResource",
		"Offset": 0,
		"Limit":  1000,
	}
	if region != "" {
		payload["Region"] = region
	}
	if projectID != "" {
		payload["ProjectId"] = projectID
	}
	if productKey != "" {
		payload["ProductKey"] = productKey
	}
	resp, _, err := d.uclient.Invoke(payload)
	if err != nil {
		return nil, err
	}
	if err := checkRetCode(resp, "ListResource"); err != nil {
		return nil, err
	}
	// 兼容多种可能的列表字段名
	list := extractList(resp, "List")
	log.DefaultLogger.Info("ListResource 解析结果",
		"listLen", len(list),
		"dataKeys", mapKeys(resp["Data"]),
		"topKeys", mapKeys(map[string]interface{}(resp)),
	)
	out := make([]Option, 0, len(list))
	for _, it := range list {
		id := asString(it["ResourceId"])
		if id == "" {
			id = asString(it["UID"])
		}
		if id == "" {
			id = asString(it["Id"])
		}
		name := asString(it["ResourceName"])
		if name == "" {
			name = asString(it["Name"])
		}
		label := id
		if name != "" {
			label = fmt.Sprintf("%s(%s)", name, id)
		}
		if id == "" {
			continue
		}
		out = append(out, Option{Text: label, Value: id})
	}
	return out, nil
}

func (d *Datasource) listAggregations() ([]Option, error) {
	resp, _, err := d.uclient.Invoke(map[string]interface{}{
		"Action": "GetMetricDataAggregationMethod",
	})
	if err != nil {
		return nil, err
	}
	if err := checkRetCode(resp, "GetMetricDataAggregationMethod"); err != nil {
		return nil, err
	}
	list := extractList(resp, "List")
	out := make([]Option, 0, len(list))
	for _, it := range list {
		value := asString(it["Value"])
		label := asString(it["Label"])
		isDefault, _ := it["IsDefault"].(bool)
		text := label
		if isDefault {
			text = label + " (默认)"
		}
		out = append(out, Option{Text: text, Value: value})
	}
	return out, nil
}

// --- 辅助函数 ---

func checkRetCode(resp map[string]interface{}, action string) error {
	code, _ := resp["RetCode"].(float64)
	if code == 0 {
		return nil
	}
	msg, _ := resp["Message"].(string)
	return fmt.Errorf("%s 失败: RetCode=%d, %s", action, int(code), msg)
}

// extractList 从响应中取出列表，兼容三种结构：
//  1. 顶层即列表，如 {"ProjectSet":[...]} （UCloud 多数 List/Describe API）
//  2. 列表嵌在 Data 下，如 {"Data":{"List":[...]}}
//  3. Data 本身是数组
func extractList(resp map[string]interface{}, listKey string) []map[string]interface{} {
	// 1) 优先尝试顶层
	if raw, ok := resp[listKey]; ok {
		if list := toMapSlice(raw); list != nil {
			return list
		}
	}
	// 顶层兜底：常见 key
	for _, key := range []string{"List", "Set", "ProjectSet", "DataSet"} {
		if raw, ok := resp[key]; ok {
			if list := toMapSlice(raw); list != nil {
				return list
			}
		}
	}

	// 2) Data 下查找
	data, ok := resp["Data"]
	if !ok {
		return nil
	}
	switch v := data.(type) {
	case map[string]interface{}:
		if raw, ok := v[listKey]; ok {
			return toMapSlice(raw)
		}
		for _, key := range []string{"List", "Set", "ProjectSet", "DataSet"} {
			if raw, ok := v[key]; ok {
				return toMapSlice(raw)
			}
		}
	case []interface{}:
		return toMapSlice(v)
	}
	return nil
}

// firstNonEmpty 返回参数中第一个非空字符串
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// mapKeys 列出 map 的所有 key（用于日志，便于看响应里 Data 都有哪些字段）
func mapKeys(v interface{}) []string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func toMapSlice(raw interface{}) []map[string]interface{} {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func asString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return ""
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// parseQuery 从 CallResourceRequest.URL（可能是 "path?query" 形式）中解析 query
func parseQuery(raw string) url.Values {
	// 兼容 URL 与 path?query
	if u, err := url.Parse(raw); err == nil {
		return u.Query()
	}
	return url.Values{}
}

// normalizeResourcePath 把 "projects"、"/projects"、"projects/" 统一为 "/projects"
func normalizeResourcePath(p string) string {
	return "/" + strings.Trim(p, "/")
}

func splitPath(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Path
	}
	return raw
}

