import React from 'react';
import * as GrafanaUI from '@grafana/ui';

type StackProps = {
  gap?: number;
  direction?: 'row' | 'column';
  alignItems?: React.CSSProperties['alignItems'];
  children?: React.ReactNode;
};

// @grafana/ui exports Stack only since Grafana 10.2. On older Grafana (e.g. 10.1.2) the
// external import resolves to undefined and React throws error #130, so fall back to a
// plain flex container. gap uses Grafana's 8px spacing grid, like the real Stack.
const FallbackStack = ({ gap = 1, direction = 'row', alignItems, children }: StackProps) => (
  <div style={{ display: 'flex', flexDirection: direction, gap: `${gap * 8}px`, alignItems }}>{children}</div>
);

export const Stack: React.ComponentType<StackProps> =
  ((GrafanaUI as unknown as Record<string, unknown>).Stack as React.ComponentType<StackProps> | undefined) ??
  FallbackStack;
