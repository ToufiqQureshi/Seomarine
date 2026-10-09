import type {
  BacklinksSortOrder,
  BacklinksTab,
  BacklinksTargetScope,
} from "@/types/schemas/backlinks";
import type {
  BacklinksOverviewData,
  BacklinksRow,
  ReferringDomainRow,
  TopPageRow,
} from "./backlinksApi";

export type {
  BacklinksOverviewData,
  BacklinksRow,
  ReferringDomainRow,
  TopPageRow,
};
export type BacklinksSearchState = {
  includeSpam?: boolean;
  target: string;
  scope: BacklinksTargetScope;
  tab: BacklinksTab;
  page: number;
  pageSize: number;
  /** Sort column id for the active tab; falls back to the tab's default. */
  sort?: string;
  order?: BacklinksSortOrder;
  /** Backlinks tab only: "all" lists every link; default is one per domain. */
  view?: "all";
};

export type BacklinksNavigate = (args: {
  search: (prev: Record<string, unknown>) => Record<string, unknown>;
  replace: boolean;
}) => void;

export type BacklinksPageProps = {
  projectId: string;
  searchState: BacklinksSearchState;
  navigate: BacklinksNavigate;
};

/** Page rows for all three tabs; tabs that haven't loaded yet are empty. */
export type BacklinksTabRows = {
  backlinks: BacklinksRow[];
  referringDomains: ReferringDomainRow[];
  topPages: TopPageRow[];
};
