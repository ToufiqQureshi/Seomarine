package gsc

import (
 "context"
 "errors"
 "net/url"
 "strings"
)

type MCPInspectionInput struct { URLs []string; LanguageCode string }
type MCPInspectionResult struct {
 URL string `json:"url"`
 Result map[string]any `json:"result,omitempty"`
 Error string `json:"error,omitempty"`
}

// InspectURLs reads the current index state using the saved project property.
func (s *Service) InspectURLs(ctx context.Context, org, project string, in MCPInspectionInput) (string, []MCPInspectionResult, error) {
 if len(in.URLs)<1 || len(in.URLs)>10 { return "",nil,validationError("urls must contain 1 to 10 items.") }
 if len(in.LanguageCode)>35 { return "",nil,validationError("languageCode is invalid.") }
 for _,raw:=range in.URLs { u,e:=url.Parse(raw); if e!=nil || (u.Scheme!="http"&&u.Scheme!="https") || u.Host=="" || u.User!=nil || len(raw)>2048 { return "",nil,validationError("Each URL must be an absolute HTTP or HTTPS URL.") } }
 connection,client,err:=s.client(ctx,org,project); if err!=nil { return "",nil,mapProviderError(err) }
 inspector,ok:=client.(urlInspector); if !ok { return "",nil,errors.New("Search Console inspection is not configured") }
 out:=make([]MCPInspectionResult,0,len(in.URLs))
 for _,raw:=range in.URLs { value,e:=inspector.InspectURL(ctx,connection.SiteURL,raw,strings.TrimSpace(in.LanguageCode)); if e!=nil { mapped:=mapProviderError(e); if _,message,ok:=PublicError(mapped);ok {out=append(out,MCPInspectionResult{URL:raw,Error:message})} else {out=append(out,MCPInspectionResult{URL:raw,Error:"Google Search Console could not inspect this URL."})}; continue }; out=append(out,MCPInspectionResult{URL:raw,Result:value}) }
 return connection.SiteURL,out,nil
}
