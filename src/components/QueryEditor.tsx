import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { getTemplateSrv } from '@grafana/runtime';
import {
  InlineField,
  MultiSelect,
  Select,
} from '@grafana/ui';

import { DataSource } from '../datasource';
import { MyDataSourceOptions, MyQuery, TagPair } from '../types';
import { MAX_RESOURCE_SELECTIONS } from '../config/enums';
import { TagsKVEditor } from './TagsKVEditor';
import { Stack } from './CompatStack';

type Props = QueryEditorProps<DataSource, MyQuery, MyDataSourceOptions>;
type Option = { text: string; value: string; globalService?: number };

// 把后端 Option[] 转成 Grafana SelectableValue 并前置 Dashboard 模板变量
function mergeVarOptions(apiList: Option[]): Array<SelectableValue<string>> {
  const vars = getTemplateSrv()
    .getVariables()
    .map((v) => ({
      label: `$${v.name}`,
      value: `$${v.name}`,
      description: '模板变量',
    }));
  const api = apiList.map((item) => ({ label: item.text, value: item.value }));
  return [...vars, ...api];
}

export function QueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const {
    projectId,
    productKey,
    region,
    metric,
    resourceIds = [],
    tags = [],
    calcMethod,
  } = query;

  const [projectOptions, setProjectOptions] = useState<Option[]>([]);
  const [productOptions, setProductOptions] = useState<Option[]>([]);
  const [regionOptions, setRegionOptions] = useState<Option[]>([]);
  const [metricOptions, setMetricOptions] = useState<Option[]>([]);
  const [resourceOptions, setResourceOptions] = useState<Option[]>([]);
  const [aggregationOptions, setAggregationOptions] = useState<Array<SelectableValue<string>>>([]);

  // 用户修改字段后再触发查询（Grafana 推荐模式）。不要在 useEffect 里自动 onRunQuery：
  // 挂载时会与 Explore 自身的首次查询并发，Grafana 13.3 Explore 的 Run query 按钮会一直转圈。
  const onKeyChange = useCallback(
    (key: keyof MyQuery, value: any) => {
      onChange({ ...query, [key]: value });
      onRunQuery();
    },
    [query, onChange, onRunQuery]
  );

  // 当前选中产品是否为全局产品（GlobalService === 1 → 隐藏地域）
  const isGlobalProduct = useMemo(
    () => productOptions.find((p) => p.value === productKey)?.globalService === 1,
    [productOptions, productKey]
  );

  // 初次加载：项目、产品、聚合方式
  useEffect(() => {
    datasource.getResource<Option[]>('/projects').then(setProjectOptions).catch(() => setProjectOptions([]));
    datasource.getResource<Option[]>('/products').then(setProductOptions).catch(() => setProductOptions([]));
    datasource
      .getResource<Array<{ text: string; value: string }>>('/aggregations')
      .then((list) => {
        const opts = (list || []).map((it) => ({ label: it.text, value: it.value }));
        setAggregationOptions(opts);
        // 默认值：标记 "(默认)" 后缀的那一项；若当前未选中则自动选
        if (!calcMethod) {
          const def = (list || []).find((it) => (it.text || '').includes('(默认)'));
          if (def) {
            onChange({ ...query, calcMethod: def.value });
          }
        }
      })
      .catch(() => setAggregationOptions([]));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [datasource]);

  // 产品变化 → 拉地域、指标
  useEffect(() => {
    if (!productKey) {
      setRegionOptions([]);
      setMetricOptions([]);
      return;
    }
    datasource
      .getResource<Option[]>(`/regions?productKey=${encodeURIComponent(productKey)}`)
      .then(setRegionOptions)
      .catch(() => setRegionOptions([]));
    datasource
      .getResource<Option[]>(`/metrics?productKey=${encodeURIComponent(productKey)}`)
      .then(setMetricOptions)
      .catch(() => setMetricOptions([]));
  }, [productKey, datasource]);

  // 项目 + 产品 + 地域（全局产品除外）都填了才拉资源
  const canLoadResources = Boolean(projectId && productKey && (isGlobalProduct || region));

  useEffect(() => {
    if (!canLoadResources) {
      setResourceOptions([]);
      return;
    }
    const params = new URLSearchParams();
    params.set('productKey', productKey!);
    const srv = getTemplateSrv();
    if (region) {
      params.set('region', srv.replace(region));
    }
    if (projectId) {
      params.set('projectId', srv.replace(projectId));
    }
    datasource
      .getResource<Option[]>(`/resources?${params.toString()}`)
      .then(setResourceOptions)
      .catch(() => setResourceOptions([]));
  }, [canLoadResources, productKey, region, projectId, datasource]);

  const projectSelectOptions = useMemo(() => mergeVarOptions(projectOptions), [projectOptions]);
  const regionSelectOptions = useMemo(() => mergeVarOptions(regionOptions), [regionOptions]);
  const resourceSelectOptions = useMemo(() => mergeVarOptions(resourceOptions), [resourceOptions]);

  const productSelectOptions = useMemo(
    () => productOptions.map((it) => ({ label: it.text, value: it.value })),
    [productOptions]
  );
  const metricSelectOptions = useMemo(
    () => metricOptions.map((it) => ({ label: it.text, value: it.value })),
    [metricOptions]
  );

  // 资源多选上限 20
  const resourceWarnRef = useRef(false);
  const onResourceChange = (values: Array<SelectableValue<string>>) => {
    if (values.length > MAX_RESOURCE_SELECTIONS) {
      if (!resourceWarnRef.current) {
        resourceWarnRef.current = true;
        window.setTimeout(() => (resourceWarnRef.current = false), 1500);
        alert(`资源对象最多选择 ${MAX_RESOURCE_SELECTIONS} 个`);
      }
      return;
    }
    onKeyChange('resourceIds', values.map((v) => v.value!).filter(Boolean));
  };

  const onTagsChange = (next: TagPair[]) => onKeyChange('tags', next);

  return (
    <Stack gap={1} direction="column">
      <InlineField label="项目" labelWidth={14} required>
        <Select
          width={40}
          placeholder="选择项目"
          value={projectId}
          options={projectSelectOptions}
          onChange={(e) => onKeyChange('projectId', e?.value ?? '')}
          isClearable
        />
      </InlineField>

      <InlineField label="产品类型" labelWidth={14} required>
        <Select
          width={40}
          placeholder="请选择产品类型"
          value={productKey}
          options={productSelectOptions}
          onChange={(e) => {
            const nextProduct = productOptions.find((p) => p.value === e?.value);
            const nextIsGlobal = nextProduct?.globalService === 1;
            // 切换产品时清空地域/指标/资源，并记录是否全局产品（用于后端校验）
            onChange({
              ...query,
              productKey: e?.value,
              isGlobalProduct: nextIsGlobal,
              region: undefined,
              metric: undefined,
              resourceIds: [],
            });
            onRunQuery();
          }}
          isSearchable
        />
      </InlineField>

      {!isGlobalProduct && (
        <InlineField label="地域" labelWidth={14} required tooltip="全局产品无需选择地域">
          <Select
            width={40}
            placeholder={productKey ? '请选择地域' : '请先选择产品类型'}
            value={region}
            options={regionSelectOptions}
            onChange={(e) => onKeyChange('region', e?.value ?? '')}
            isClearable
            disabled={!productKey}
          />
        </InlineField>
      )}

      <InlineField label="指标" labelWidth={14} required>
        <Select
          width={40}
          placeholder={productKey ? '请选择指标' : '请先选择产品类型'}
          value={metric}
          options={metricSelectOptions}
          onChange={(e) => onKeyChange('metric', e?.value ?? '')}
          isSearchable
          disabled={!productKey}
        />
      </InlineField>

      <InlineField
        label="资源对象"
        labelWidth={14}
        required
        tooltip={`最多选择 ${MAX_RESOURCE_SELECTIONS} 个资源`}
      >
        <MultiSelect
          width={60}
          placeholder={canLoadResources ? '请选择资源' : '请先选择项目、产品类型和地域'}
          value={resourceIds}
          options={resourceSelectOptions}
          onChange={onResourceChange}
          disabled={!canLoadResources}
        />
      </InlineField>

      <InlineField label="数据维度" labelWidth={14} grow>
        <TagsKVEditor value={tags} onChange={onTagsChange} />
      </InlineField>

      <InlineField label="聚合方式" labelWidth={14}>
        <Select
          width={40}
          placeholder="请选择聚合方式"
          value={calcMethod}
          options={aggregationOptions}
          onChange={(e) => onKeyChange('calcMethod', e?.value ?? '')}
        />
      </InlineField>
    </Stack>
  );
}
