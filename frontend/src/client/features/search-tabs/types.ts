import type {
  KeywordMode,
  ResultLimit,
} from "@/client/features/keywords/keywordResearchTypes";
import type { ResearchScope } from "@/shared/researchScope";

export type BacklinksSearchTabInput = {
  type: "backlinks";
  target: string;
  scope: ResearchScope;
};

export type DomainSearchTabInput = {
  type: "domain";
  domain: string;
  scope: ResearchScope;
  locationCode?: number;
};

export type KeywordSearchTabInput = {
  type: "keyword";
  keyword: string;
  locationCode?: number;
  /** City, county, or region for local volume; absent for national. */
  locationName?: string;
  resultLimit: ResultLimit;
  mode: KeywordMode;
  clickstream: boolean;
  groupKeywords: boolean;
};

export type SearchTabInput =
  | BacklinksSearchTabInput
  | DomainSearchTabInput
  | KeywordSearchTabInput;

export type SearchTab = {
  id: string;
  label: string;
  input: SearchTabInput;
  createdAt: number;
  viewedAt: number | null;
};
