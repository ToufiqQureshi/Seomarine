package mcp

import (
 "context"
 "encoding/json"
 "fmt"
 "strings"
 "time"

 "github.com/toufiqqureshi/seomarine/backend/internal/keywords"
)

func listBusinessCategoriesTool()*tool{return &tool{Name:"list_business_categories",Title:"List business categories",Description:"Lists Google Business categories recognized by DataForSEO, ranked by business count. Uses no credits.",
 InputSchema:json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"query":{"type":"string","minLength":1,"maxLength":80},"limit":{"type":"integer","minimum":1,"maximum":200}},"required":["projectId"],"additionalProperties":false}`),
 OutputSchema:json.RawMessage(`{"type":"object","properties":{"categories":{"type":"array","items":{"type":"object","properties":{"category":{"type":"string"},"businessCount":{"type":["number","null"]}}}}},"additionalProperties":true}`),Annotations:readOnlyAnnotations(),Handler:handleListBusinessCategories}}
func handleListBusinessCategories(ctx context.Context,raw json.RawMessage,env *callEnv)(*callResult,error){
 var a struct{ProjectID string `json:"projectId"`;Query string `json:"query"`;Limit int `json:"limit"`}
 if e:=json.Unmarshal(raw,&a);e!=nil||strings.TrimSpace(a.ProjectID)==""{return nil,newAppErrorf("VALIDATION_ERROR","projectId is required")}
 if a.Query!=""&&(len([]rune(a.Query))>80){return nil,newAppErrorf("VALIDATION_ERROR","query must be at most 80 characters")}
 limit:=a.Limit;if limit==0{limit=50};if limit<1||limit>200{return nil,newAppErrorf("VALIDATION_ERROR","limit must be between 1 and 200")}
 access,e:=env.h.authorizeProject(ctx,env.auth,a.ProjectID);if e!=nil{return nil,e}
 if env.deps.KeywordResearch==nil{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Business categories are not configured on this server.")}
 provider,ok:=env.deps.KeywordResearch.Data.(keywords.BusinessCategoriesProvider);if !ok{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Business categories are not configured on this server.")}
 const key="mcp:business-categories:v1";const ttl=7*24*time.Hour
 var all []keywords.BusinessCategory
 if env.deps.Redis!=nil{cached,err:=env.deps.Redis.Get(ctx,key).Bytes();if err==nil{_ = json.Unmarshal(cached,&all)}}
 if all==nil{all,e=provider.BusinessCategories(ctx);if e!=nil{env.deps.Logger.ErrorContext(ctx,"MCP business categories","project_id",access.Project.ID,"err",e);return nil,newAppErrorf("INTERNAL_ERROR","Unable to load business categories.")};if all==nil{all=[]keywords.BusinessCategory{}};if env.deps.Redis!=nil{if data,err:=json.Marshal(all);err==nil{if err:=env.deps.Redis.Set(ctx,key,data,ttl).Err();err!=nil{env.deps.Logger.WarnContext(ctx,"cache business categories","err",err)}}}}
 query:=strings.ToLower(strings.TrimSpace(a.Query));matched:=make([]keywords.BusinessCategory,0);for _,row:=range all{if query==""||strings.Contains(strings.ToLower(row.Category),query){matched=append(matched,row)}}
 count:=len(matched);if len(matched)>limit{matched=matched[:limit]}
 lines:=[]string{fmt.Sprintf("Found %d categories%s; showing %d.",count,func()string{if a.Query!=""{return fmt.Sprintf(" matching %q",a.Query)};return ""}(),len(matched))}
 for _,row:=range matched{lines=append(lines,fmt.Sprintf("%s | %s",row.Category,nullableMetric(row.BusinessCount)))}
 return mcpResponse(strings.Join(lines,"\n"),map[string]any{"categories":matched},metaFields{ProjectID:access.Project.ID,URL:buildDashboardURL(env.auth.BaseURL,"/p/"+access.Project.ID,nil)}),nil
}
