import {ApiError} from "./api";

// Turns thrown values into user-facing copy: ApiError already carries a
// humanized server message; everything else is classified by shape.
export function friendlyError(e:unknown):string{
  if(e instanceof ApiError)return e.message;
  const raw=String(e);
  if(/Failed to fetch|NetworkError|network|load failed/i.test(raw))return "网络不可用，请检查连接后重试。";
  if(/abort/i.test(raw))return "请求超时，请稍后重试。";
  return "请求失败，请稍后重试。";
}
