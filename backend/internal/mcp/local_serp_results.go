package mcp

import (
 "context"
 "encoding/json"
 "fmt"
 "strings"

 "github.com/toufiqqureshi/seomarine/backend/internal/keywords"
)

var localSERPFields=[]string{"rank_group","rank_absolute","title","domain","url","contact_url","address","address_info","phone","category","additional_categories","rating","rating_distribution","price_level","is_claimed","cid","place_id","latitude","longitude","total_photos","work_hours","local_justifications"}
func getLocalSerpResultsTool()*tool{return &tool{Name:"get_local_serp_results",Title:"Get local SERP results",Description:"Fetch one Google Maps or Local Finder SERP near a coordinate. Returns trimmed provider rows with rank fields intact. Charges credits.",
 InputSchema:json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"keyword":{"type":"string","minLength":1,"maxLength":120},"near":{"type":"object","properties":{"latitude":{"type":"number","minimum":-90,"maximum":90},"longitude":{"type":"number","minimum":-180,"maximum":180},"zoom":{"type":"integer","minimum":4,"maximum":18}},"required":["latitude","longitude"],"additionalProperties":false},"searchType":{"type":"string","enum":["maps","local_finder"]},"device":{"type":"string","enum":["desktop","mobile"]},"depth":{"type":"integer","minimum":1,"maximum":100},"languageCode":{"type":"string","minLength":2,"maxLength":8}},"required":["projectId","keyword","near"],"additionalProperties":false}`),
 OutputSchema:json.RawMessage(`{"type":"object","properties":{"results":{"type":"array","items":{"type":"object"}}},"additionalProperties":true}`),Annotations:map[string]any{"readOnlyHint":false,"openWorldHint":true,"destructiveHint":false},Handler:handleGetLocalSerpResults}}
func handleGetLocalSerpResults(ctx context.Context,raw json.RawMessage,env *callEnv)(*callResult,error){
 var a struct{ProjectID string `json:"projectId"`;Keyword string `json:"keyword"`;Near struct{Latitude *float64 `json:"latitude"`;Longitude *float64 `json:"longitude"`;Zoom *int `json:"zoom"`} `json:"near"`;SearchType string `json:"searchType"`;Device string `json:"device"`;Depth int `json:"depth"`;LanguageCode string `json:"languageCode"`}
 if e:=json.Unmarshal(raw,&a);e!=nil||strings.TrimSpace(a.ProjectID)==""||strings.TrimSpace(a.Keyword)==""{return nil,newAppErrorf("VALIDATION_ERROR","projectId, keyword, and near are required")}
 if len(a.Keyword)>120||a.Near.Latitude==nil||a.Near.Longitude==nil||*a.Near.Latitude < -90||*a.Near.Latitude>90||*a.Near.Longitude < -180||*a.Near.Longitude>180{return nil,newAppErrorf("VALIDATION_ERROR","keyword or near coordinates are invalid")}
 searchType:=a.SearchType;if searchType==""{searchType="maps"};if searchType!="maps"&&searchType!="local_finder"{return nil,newAppErrorf("VALIDATION_ERROR","searchType must be maps or local_finder")}
 device:=a.Device;if device==""{device="mobile"};if device!="mobile"&&device!="desktop"{return nil,newAppErrorf("VALIDATION_ERROR","device must be mobile or desktop")}
 depth:=a.Depth;if depth==0{depth=20};if depth<1||depth>100{return nil,newAppErrorf("VALIDATION_ERROR","depth must be from 1 to 100")}
 if a.Near.Zoom!=nil&&(*a.Near.Zoom<4||*a.Near.Zoom>18){return nil,newAppErrorf("VALIDATION_ERROR","zoom must be between 4 and 18")}
 if a.LanguageCode!=""&&(len(strings.TrimSpace(a.LanguageCode))<2||len(strings.TrimSpace(a.LanguageCode))>8){return nil,newAppErrorf("VALIDATION_ERROR","languageCode must be 2 to 8 characters")}
 access,e:=env.h.authorizeProject(ctx,env.auth,a.ProjectID);if e!=nil{return nil,e}
 if env.deps.KeywordResearch==nil{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Keyword research provider is not configured.")}
 provider,ok:=env.deps.KeywordResearch.Data.(keywords.LocalSERPProvider);if !ok{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Local SERP provider is not configured.")}
 language:=strings.TrimSpace(a.LanguageCode);if language==""{language=access.Project.LanguageCode}
 rows,e:=provider.LocalSERP(ctx,access.Auth.OrganizationID,keywords.LocalSERPInput{Keyword:strings.TrimSpace(a.Keyword),Coordinate:keywords.FormatLocalSERPCoordinate(*a.Near.Latitude,*a.Near.Longitude,a.Near.Zoom),LanguageCode:language,SearchType:searchType,Device:device,Depth:depth})
 if e!=nil{env.deps.Logger.ErrorContext(ctx,"MCP local SERP","project_id",access.Project.ID,"err",e);return nil,newAppErrorf("INTERNAL_ERROR","Unable to fetch local SERP results.")}
 results:=make([]map[string]any,0,len(rows));for _,row:=range rows{trimmed:=map[string]any{};for _,field:=range localSERPFields{if v,ok:=row[field];ok{trimmed[field]=v}};results=append(results,trimmed)}
 header:=fmt.Sprintf("Fetched %d local SERP rows for %q.",len(results),a.Keyword);lines:=[]string{header}
 for _,r:=range results{rank:=r["rank_absolute"];if rank==nil{rank=r["rank_group"]};rating,_:=r["rating"].(map[string]any);lines=append(lines,fmt.Sprintf("%v | %v | %v | %v | %v | %v",rank,r["title"],rating["value"],rating["votes_count"],r["phone"],r["address"]))}
 return mcpResponse(strings.Join(lines,"\n"),map[string]any{"results":results},metaFields{ProjectID:access.Project.ID,URL:buildDashboardURL(env.auth.BaseURL,"/p/"+access.Project.ID,nil)}),nil
}
