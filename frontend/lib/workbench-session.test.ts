import { afterEach, expect, it, vi } from "vitest";
import { establishWorkbenchSession } from "./workbench-session";
afterEach(()=>vi.unstubAllGlobals());
it("establishes a CSRF-bound same-origin session without returning a token",async()=>{document.cookie="oasis_csrf=csrf%20value";const fetch=vi.fn(async()=>new Response(JSON.stringify({workspace_id:"w",expires_in:300}),{status:200}));vi.stubGlobal("fetch",fetch);await expect(establishWorkbenchSession()).resolves.toEqual({workspace_id:"w",expires_in:300});expect(fetch).toHaveBeenCalledWith("/api/workbench/session",expect.objectContaining({method:"POST",credentials:"include",headers:expect.objectContaining({"x-csrf-token":"csrf value"})}));expect(JSON.stringify(await establishWorkbenchSession())).not.toContain("token")});
it("fails closed when the session endpoint fails",async()=>{vi.stubGlobal("fetch",vi.fn(async()=>new Response("",{status:401})));await expect(establishWorkbenchSession()).rejects.toThrow("unavailable")});
