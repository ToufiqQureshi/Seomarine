package mcp

import (
 "context"
 "encoding/json"
 "fmt"
 "sort"
 "strings"

 "github.com/toufiqqureshi/seomarine/backend/internal/domain"
 "github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

func findSerpCompetitorsTool()*tool{return &tool{Name:"find_serp_competitors",Title:"Find SERP competitors",
 Description:"Compares domains competing in Google results for supplied keywords in a country-level DataForSEO Labs market. This is not radius-based local SEO. Charges credits.",
 InputSchema:json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"market":{"type":"object","properties":{"country":{"type":"string","enum":["US","USA","United States","United States of America"]}}},"keywords":{"type":"array","items":{"type":"string","minLength":1,"maxLength":120},"minItems":1,"maxItems":100},"locationCode":{"type":"integer","minimum":1},"languageCode":{"type":"string","minLength":2,"maxLength":8},"resultTypes":{"type":"array","items":{"type":"string","enum":["organic","paid","featured_snippet","local_pack"]},"minItems":1,"maxItems":4},"excludeDomains":{"type":"array","items":{"type":"string","minLength":1,"maxLength":255},"minItems":1,"maxItems":50},"includeSubdomains":{"type":"boolean"},"sortBy":{"type":"string","enum":["visibility","traffic_estimate","avg_position","keyword_count"]},"limit":{"type":"integer","minimum":1,"maximum":100},"offset":{"type":"integer","minimum":0,"maximum":1000}},"required":["projectId","keywords"],"additionalProperties":false}`),
 OutputSchema:json.RawMessage(`{"type":"object","properties":{"competitors":{"type":"array","items":{"type":"object"}}},"additionalProperties":true}`),
 Annotations:map[string]any{"readOnlyHint":false,"openWorldHint":true,"destructiveHint":false},Handler:handleFindSerpCompetitors}}
func handleFindSerpCompetitors(ctx context.Context,raw json.RawMessage,env *callEnv)(*callResult,error){
 var a struct{ProjectID string `json:"projectId"`;Market *struct{Country *string `json:"country"`} `json:"market"`;Keywords []string `json:"keywords"`;LocationCode *int `json:"locationCode"`;LanguageCode string `json:"languageCode"`;ResultTypes []string `json:"resultTypes"`;ExcludeDomains []string `json:"excludeDomains"`;IncludeSubdomains *bool `json:"includeSubdomains"`;SortBy string `json:"sortBy"`;Limit int `json:"limit"`;Offset int `json:"offset"`}
 if e:=json.Unmarshal(raw,&a);e!=nil||strings.TrimSpace(a.ProjectID)==""{return nil,newAppErrorf("VALIDATION_ERROR","projectId and keywords are required")}
 if len(a.Keywords)<1||len(a.Keywords)>100{return nil,newAppErrorf("VALIDATION_ERROR","keywords must contain 1 to 100 values")}
 for _,k:=range a.Keywords{if strings.TrimSpace(k)==""||len([]rune(k))>120{return nil,newAppErrorf("VALIDATION_ERROR","each keyword must contain 1 to 120 characters")}}
 if a.Market!=nil&&a.Market.Country!=nil{switch *a.Market.Country{case "US","USA","United States","United States of America":default:return nil,newAppErrorf("VALIDATION_ERROR","Only the United States can be selected explicitly.")}}
 if a.LocationCode!=nil&&*a.LocationCode<1{return nil,newAppErrorf("VALIDATION_ERROR","locationCode must be positive")}
 if a.LanguageCode!=""&&(len(strings.TrimSpace(a.LanguageCode))<2||len(strings.TrimSpace(a.LanguageCode))>8){return nil,newAppErrorf("VALIDATION_ERROR","languageCode must be 2 to 8 characters")}
 if a.ResultTypes!=nil&&(len(a.ResultTypes)<1||len(a.ResultTypes)>4){return nil,newAppErrorf("VALIDATION_ERROR","resultTypes must contain 1 to 4 values")}
 if a.ExcludeDomains!=nil&&(len(a.ExcludeDomains)<1||len(a.ExcludeDomains)>50){return nil,newAppErrorf("VALIDATION_ERROR","excludeDomains must contain 1 to 50 domains")}
 for _,d:=range a.ExcludeDomains{if len(d)>255||!domain.ValidMCPCompetitorDomain(d){return nil,newAppErrorf("VALIDATION_ERROR","excludeDomains must contain domains without protocol or www")}}
 switch a.SortBy{case "","visibility","traffic_estimate","avg_position","keyword_count":default:return nil,newAppErrorf("VALIDATION_ERROR","sortBy is invalid")}
 if env.deps.Domain==nil{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Domain analysis is not configured on this server.")}
 access,e:=env.h.authorizeProject(ctx,env.auth,a.ProjectID);if e!=nil{return nil,e}
 code:=0;if a.LocationCode!=nil{code=*a.LocationCode}
 requested:=market.Pair{LocationCode:code,LanguageCode:strings.TrimSpace(a.LanguageCode)}
 if code==0&&a.LanguageCode==""&&a.Market!=nil&&a.Market.Country!=nil{requested=market.Pair{LocationCode:2840,LanguageCode:"en"}}
 pair:=market.ResolveLabs(requested,market.Pair{LocationCode:access.Project.LocationCode,LanguageCode:access.Project.LanguageCode})
 if !market.IsLabsLocationCode(pair.LocationCode){return nil,newAppErrorf("VALIDATION_ERROR","SERP competitor research is unavailable for this country.")}
 if !market.IsLanguageServedForLocation(pair.LocationCode,pair.LanguageCode){return nil,newAppErrorf("VALIDATION_ERROR","Language is not available for this location.")}
 rows,e:=env.deps.Domain.SerpCompetitorsForMCP(ctx,access.Auth.OrganizationID,domain.MCPSerpCompetitorInput{Keywords:a.Keywords,LocationCode:pair.LocationCode,LanguageCode:pair.LanguageCode,ItemTypes:a.ResultTypes,IncludeSubdomains:a.IncludeSubdomains,Limit:a.Limit,Offset:a.Offset})
 if e!=nil{if domain.IsBadRequest(e){return nil,newAppErrorf("VALIDATION_ERROR",e.Error())};env.deps.Logger.ErrorContext(ctx,"MCP SERP competitors","project_id",access.Project.ID,"err",e);return nil,newAppErrorf("INTERNAL_ERROR","Unable to fetch SERP competitors.")}
 filtered:=rows[:0]
 for _,row:=range rows{host:="";if row.Domain!=nil{host=strings.ToLower(strings.TrimPrefix(*row.Domain,"www."))};excluded:=false;for _,d:=range a.ExcludeDomains{d=strings.ToLower(strings.TrimPrefix(d,"www."));if host==d||strings.HasSuffix(host,"."+d){excluded=true;break}};if !excluded{filtered=append(filtered,row)}}
 field:=a.SortBy;if field==""{field="visibility"}
 get:=func(r domain.MCPSerpCompetitor)float64{switch field{case "avg_position":if r.AvgPosition!=nil{return *r.AvgPosition};case "keyword_count":if r.KeywordsCount!=nil{return *r.KeywordsCount};case "traffic_estimate":if r.ETV!=nil{return *r.ETV};default:if r.Visibility!=nil{return *r.Visibility}};return 0}
 direction:=1.0;if field!="avg_position"{direction=-1};sort.SliceStable(filtered,func(i,j int)bool{return (get(filtered[i])-get(filtered[j]))*direction<0})
 lines:=[]string{fmt.Sprintf("Found %d SERP competitors across %d keywords.",len(filtered),len(a.Keywords))};for _,r:=range filtered{lines=append(lines,fmt.Sprintf("%s | %s | %s | %s | %s | %s",stringValue(r.Domain),nullableMetric(r.KeywordsCount),nullableMetric(r.AvgPosition),nullableMetric(r.MedianPosition),nullableMetric(r.Visibility),nullableMetric(r.ETV)))}
 return mcpResponse(strings.Join(lines,"\n"),map[string]any{"competitors":filtered},metaFields{ProjectID:access.Project.ID,URL:buildDashboardURL(env.auth.BaseURL,"/p/"+access.Project.ID+"/domain",nil)}),nil
}
