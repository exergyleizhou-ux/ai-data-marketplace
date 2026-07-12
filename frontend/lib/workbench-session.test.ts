import { afterEach, expect, it, vi } from "vitest";
import { establishWorkbenchSession } from "./workbench-session";
afterEach(()=>vi.unstubAllGlobals());
it("establishes a same-origin credentialed session without returning a token",async()=>{const fetch=vi.fn(async()=>new Response(JSON.stringify({workspace_id:"w",expires_in:300}),{status:200}));vi.stubGlobal("fetch",fetch);await expect(establishWorkbenchSession()).resolves.toEqual({workspace_id:"w",expires_in:300});expect(fetch).toHaveBeenCalledWith("/api/workbench/session",expect.objectContaining({method:"POST",credentials:"include"}));expect(JSON.stringify(await establishWorkbenchSession())).not.toContain("token")});
it("fails closed when the session endpoint fails",async()=>{vi.stubGlobal("fetch",vi.fn(async()=>new Response("",{status:401})));await expect(establishWorkbenchSession()).rejects.toThrow("unavailable")});
