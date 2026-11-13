export interface Column<R> {
  key: string;
  label: string;
  value: (r: R) => string | number | null;
  cls?: string;
  numeric?: boolean;
  /** Explanation shown on the column header. */
  title?: string;
  /** false: the column cannot be sorted (e.g. a chart). */
  sortable?: boolean;
}
