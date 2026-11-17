import React from 'react';
import { Button, IconButton, Input } from '@grafana/ui';
import { Stack } from './CompatStack';
import { TagPair } from '../types';

interface Props {
  value: TagPair[];
  onChange: (next: TagPair[]) => void;
}

// 数据维度 KV 编辑器：一行一组 key + values（values 多值用逗号分隔）
export function TagsKVEditor({ value, onChange }: Props) {
  const rows = value.length > 0 ? value : [{ key: '', values: [] }];

  const updateRow = (index: number, patch: Partial<TagPair>) => {
    const next = rows.map((row, i) => (i === index ? { ...row, ...patch } : row));
    onChange(next);
  };

  const removeRow = (index: number) => {
    const next = rows.filter((_, i) => i !== index);
    onChange(next);
  };

  const addRow = () => {
    onChange([...rows, { key: '', values: [] }]);
  };

  return (
    <Stack gap={0.5} direction="column">
      {rows.map((row, index) => (
        <Stack key={index} gap={0.5} direction="row" alignItems="center">
          <Input
            width={24}
            placeholder="Key"
            value={row.key}
            onChange={(e) => updateRow(index, { key: (e.target as HTMLInputElement).value })}
          />
          <Input
            width={36}
            placeholder="Value（多值逗号分隔）"
            value={row.values.join(',')}
            onChange={(e) =>
              updateRow(index, {
                values: (e.target as HTMLInputElement).value
                  .split(',')
                  .map((s) => s.trim())
                  .filter(Boolean),
              })
            }
          />
          <IconButton
            name="trash-alt"
            aria-label="删除"
            tooltip="删除该维度"
            onClick={() => removeRow(index)}
          />
        </Stack>
      ))}
      <div>
        <Button icon="plus" size="sm" variant="secondary" onClick={addRow}>
          添加
        </Button>
      </div>
    </Stack>
  );
}
