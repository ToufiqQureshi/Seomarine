package mcp

import (
 "context"
 "encoding/json"
 "fmt"
 "strings"

 "github.com/toufiqqureshi/seomarine/backend/internal/keywords"
 "github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

type researchKeywordRow struct {
 Keyword string `json:"keyword"`
 SearchVolume *int `json:"searchVolume"`
 KeywordDifficulty *int `json:"keywordDifficulty"`
 CPC *float64 `json:"cpc"`
 Competition *float64 `json:"competition"`
 Intent string `json:"intent"`
}
type researchSeedResult struct {
 Seed string `json:"seed"`
 OK bool `json:"ok"`
 LocationName *string `json:"locationName,omitempty"`
 RowCount int `json:"rowCount,omitempty"`
 Source string `json:"source,omitempty"`
 UsedFallback bool `json:"usedFallback,omitempty"`
 Rows []researchKeywordRow `json:"rows,omitempty"`
 Error string `json:"error,omitempty"`
}
func researchKeywordsTool() *tool {
 return &tool{Name:"research_keywords",Title:"Research keywords (bulk)",
 Description:"Research keyword data (search volume, difficulty, CPC, related ideas) for 1-5 seed keywords in one call. Each seed returns independently, so one bad seed does not fail the batch. Charges credits.",
 InputSchema:json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"seeds":{"type":"array","minItems":1,"maxItems":5,"items":{"type":"object","properties":{"seed":{"type":"string","minLength":1},"locationCode":{"type":"integer","minimum":1},"languageCode":{"type":"string","minLength":2,"maxLength":8},"locationName":{"type":"string","minLength":1}},"required":["seed"],"additionalProperties":false}},"resultLimit":{"type":"integer","enum":[150,300,500]},"includeClickstreamData":{"type":"boolean"},"groupKeywords":{"type":"boolean"}},"required":["projectId","seeds"],"additionalProperties":false}`),
 OutputSchema:json.RawMessage(`{"type":"object","properties":{"results":{"type":"array","items":{"type":"object"}}},"additionalProperties":true}`),
 Annotations:map[string]any{"readOnlyHint":false,"openWorldHint":true,"destructiveHint":false},Handler:handleResearchKeywords}
}
func handleResearchKeywords(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult,error) {
 var args struct {
  ProjectID string `json:"projectId"`
  Seeds []struct { Seed string `json:"seed"`; LocationCode *int `json:"locationCode"`; LanguageCode string `json:"languageCode"`; LocationName *string `json:"locationName"` } `json:"seeds"`
  ResultLimit int `json:"resultLimit"`; IncludeClickstreamData bool `json:"includeClickstreamData"`; GroupKeywords bool `json:"groupKeywords"`
 }
 if err:=json.Unmarshal(raw,&args); err!=nil || strings.TrimSpace(args.ProjectID)=="" { return nil,newAppErrorf("VALIDATION_ERROR","projectId and seeds are required") }
 if len(args.Seeds)<1 || len(args.Seeds)>5 { return nil,newAppErrorf("VALIDATION_ERROR","seeds must contain 1 to 5 items") }
 limit:=args.ResultLimit; if limit==0 {limit=150}; if limit!=150 && limit!=300 && limit!=500 {return nil,newAppErrorf("VALIDATION_ERROR","resultLimit must be 150, 300, or 500")}
 for _,s:=range args.Seeds { if strings.TrimSpace(s.Seed)=="" {return nil,newAppErrorf("VALIDATION_ERROR","each seed keyword is required")}; if s.LocationCode!=nil && *s.LocationCode<1 {return nil,newAppErrorf("VALIDATION_ERROR","locationCode must be positive")}; if s.LanguageCode!="" && (len(strings.TrimSpace(s.LanguageCode))<2 || len(strings.TrimSpace(s.LanguageCode))>8) {return nil,newAppErrorf("VALIDATION_ERROR","languageCode must be 2 to 8 characters")}; if s.LocationName!=nil && strings.TrimSpace(*s.LocationName)=="" {return nil,newAppErrorf("VALIDATION_ERROR","locationName must not be empty")} }
 if env.deps.KeywordResearch==nil {return nil,newAppErrorf("SERVICE_UNAVAILABLE","Keyword research is not configured on this server.")}
 access,err:=env.h.authorizeProject(ctx,env.auth,args.ProjectID); if err!=nil {return nil,err}
 results:=make([]researchSeedResult,0,len(args.Seeds)); okCount:=0
 for _,seed:=range args.Seeds {
  result:=researchSeedResult{Seed:seed.Seed,LocationName:seed.LocationName}; code:=0; if seed.LocationCode!=nil {code=*seed.LocationCode}
  pair,e:=keywords.ResolveMarket(code,strings.TrimSpace(seed.LanguageCode),market.Pair{LocationCode:access.Project.LocationCode,LanguageCode:access.Project.LanguageCode})
  if e==nil && !market.IsLanguageServedForLocation(pair.LocationCode,pair.LanguageCode) {e=keywords.ValidationError("Language is not available for this location.")}
  if e!=nil {result.Error=e.Error();results=append(results,result);continue}
  data,e:=env.deps.KeywordResearch.Research(ctx,keywords.ResearchInput{OrganizationID:access.Auth.OrganizationID,ProjectID:access.Project.ID,Keywords:[]string{seed.Seed},LocationCode:pair.LocationCode,LanguageCode:pair.LanguageCode,LocationName:seed.LocationName,ResultLimit:limit,Mode:"auto",Clickstream:args.IncludeClickstreamData,GroupKeywords:args.GroupKeywords})
  if e!=nil {result.Error=e.Error();results=append(results,result);continue}
  result.OK=true; result.RowCount=len(data.Rows);result.Source=data.Source;result.UsedFallback=data.UsedFallback;result.Rows=make([]researchKeywordRow,0,len(data.Rows))
  for _,r:=range data.Rows {result.Rows=append(result.Rows,researchKeywordRow{Keyword:r.Keyword,SearchVolume:r.SearchVolume,KeywordDifficulty:r.KeywordDifficulty,CPC:r.CPC,Competition:r.Competition,Intent:r.Intent})}
  results=append(results,result);okCount++
 }
 lines:=make([]string,0,len(results)+1)
 for _,r:=range results {
  if !r.OK {lines=append(lines,fmt.Sprintf("## %q — FAILED\n%s",r.Seed,r.Error));continue}
  head:=fmt.Sprintf("## %q — %d keywords (source: %s",r.Seed,r.RowCount,r.Source); if r.UsedFallback {head+=", fallback"};if r.LocationName!=nil {head+=fmt.Sprintf(", volume/CPC/competition for %s, KD/intent national",*r.LocationName)};head+=")"
  if r.RowCount==0 {lines=append(lines,head+"\n(no keywords returned)");continue}
  table:=[]string{"keyword | volume | KD | CPC | competition | intent"};for _,v:=range r.Rows {table=append(table,fmt.Sprintf("%s | %s | %s | %s | %s | %s",v.Keyword,nullableMetric(v.SearchVolume),nullableMetric(v.KeywordDifficulty),nullableMetric(v.CPC),nullableMetric(v.Competition),v.Intent))}
  lines=append(lines,head+"\n"+strings.Join(table,"\n"))
 }
 failed:=len(results)-okCount;summary:=fmt.Sprintf("Researched %d of %d seeds",okCount,len(results));if failed>0 {summary+=fmt.Sprintf(" (%d failed)",failed)};summary+=". Columns: volume = monthly searches, KD = keyword difficulty (0-100), CPC in USD, competition = paid competition (0-1); \"—\" = unavailable. Google Ads close variants may share search volumes; do not add their volumes together."
 lines=append(lines,summary)
 return mcpResponse(strings.Join(lines,"\n\n"),map[string]any{"results":results},metaFields{ProjectID:access.Project.ID,URL:buildDashboardURL(env.auth.BaseURL,"/p/"+access.Project.ID+"/keywords",nil)}),nil
}
