export interface Column<R> {
  key: string;
  label: string;
  value: (r: R) => string | number | null;
  cls?: string;
  numeric?: boolean;
}
