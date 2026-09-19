/** The table columns a user can show or hide. Session and Actions always show. */
export type ColumnKey = "profile" | "region" | "state";
export type Columns = Record<ColumnKey, boolean>;
export const COLUMN_KEYS: ColumnKey[] = ["profile", "region", "state"];
export const DEFAULT_COLUMNS: Columns = { profile: true, region: true, state: true };
