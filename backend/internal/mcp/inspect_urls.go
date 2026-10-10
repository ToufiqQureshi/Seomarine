package mcp

import (
 "context"
 "encoding/json"
 "fmt"
 "net/url"
 "strings"

 "github.com/toufiqqureshi/seomarine/backend/internal/gsc"
)

func inspectURLsTool()*tool { return &tool{
 Name:"inspect_urls",Title:"Inspect URLs in Google Search Console",
 Description:"Read Google Search Console's existing index status for up to 10 URLs of the connected property. Per-URL errors are reported inline. Read-only; uses no credits.",
 InputSchema:json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"urls":{"type":"array","items":{"type":"string","format":"uri"},"minItems":1,"maxItems":10},"languageCode":{"type":"string"}},"required":["projectId","urls"],"additionalProperties":false}`),
 OutputSchema:json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"},"reason":{"type":"string"},"connectUrl":{"type":"string"},"siteUrl":{"type":"string"},"results":{"type":"array","items":{"type":"object"}}},"required":["ok"],"additionalProperties":true}`),
 Annotations:readOnlyAnnotations(),Handler:handleInspectURLs,
}}
func handleInspectURLs(ctx context.Context,raw json.RawMessage,env *callEnv)(*callResult,error) {
 var a struct{ ProjectID string `json:"projectId"`; URLs []string `json:"urls"`; LanguageCode string `json:"languageCode"` }
 if e:=json.Unmarshal(raw,&a); e!=nil || strings.TrimSpace(a.ProjectID)=="" {return nil,newAppErrorf("VALIDATION_ERROR","projectId and urls are required")}
 if len(a.URLs)<1 || len(a.URLs)>10 {return nil,newAppErrorf("VALIDATION_ERROR","urls must contain 1 to 10 items")}
 for _,raw:=range a.URLs {u,e:=url.Parse(raw);if e!=nil||(u.Scheme!="http"&&u.Scheme!="https")||u.Host==""||u.User!=nil{return nil,newAppErrorf("VALIDATION_ERROR","Each URL must be an absolute HTTP or HTTPS URL.")}}
 access,e:=env.h.authorizeProject(ctx,env.auth,a.ProjectID);if e!=nil{return nil,e}
 meta:=metaFields{ProjectID:access.Project.ID,URL:buildDashboardURL(env.auth.BaseURL,"/p/"+access.Project.ID+"/settings/integrations",nil)}
 connect:=connectSearchConsoleURL(env.auth.BaseURL,access.Project.ID)
 if env.deps.GSC==nil{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Search Console is not available on this server.")}
 site,results,e:=env.deps.GSC.InspectURLs(ctx,access.Project.OrganizationID,access.Project.ID,gsc.MCPInspectionInput{URLs:a.URLs,LanguageCode:a.LanguageCode})
 if e!=nil {if gsc.IsValidationError(e){return nil,newAppErrorf("INVALID_REQUEST",e.Error())};if code,msg,ok:=gsc.PublicError(e);ok{return mcpResponse(msg+" (reconnect at "+connect+")",map[string]any{"ok":false,"reason":code,"connectUrl":connect},meta),nil};env.deps.Logger.ErrorContext(ctx,"MCP GSC URL inspection","project_id",access.Project.ID,"err",e);return mcpResponse("Search Console URL inspection is temporarily unavailable. (reconnect at "+connect+")",map[string]any{"ok":false,"reason":"api_error","connectUrl":connect},meta),nil}
 lines:=[]string{fmt.Sprintf("%s · inspected %d URLs",site,len(results))}
 for _,r:=range results {if r.Error!=""{lines=append(lines,"  "+r.URL+" — error: "+r.Error);continue};status,_:=r.Result["indexStatusResult"].(map[string]any);verdict,_:=status["verdict"].(string);if verdict==""{verdict="UNKNOWN"};coverage,_:=status["coverageState"].(string);if coverage==""{coverage="—"};canonical,_:=status["googleCanonical"].(string);if canonical!=""{coverage+=", google-canonical "+canonical};lines=append(lines,"  "+r.URL+" — "+verdict+": "+coverage)}
 return mcpResponse(strings.Join(lines,"\n"),map[string]any{"ok":true,"siteUrl":site,"results":results},meta),nil
}
