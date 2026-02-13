/**
 * Build a canonical payload string for device auth signing.
 * Must match backend expectation (e.g. JSON or concatenated fields).
 */
export function buildDeviceAuthPayload(params: {
  deviceId: string;
  clientId: string;
  clientMode: string;
  role: string;
  scopes: string[];
  signedAtMs: number;
  token: string | null;
  nonce?: string | null;
}): string {
  const obj = {
    deviceId: params.deviceId,
    clientId: params.clientId,
    clientMode: params.clientMode,
    role: params.role,
    scopes: params.scopes,
    signedAtMs: params.signedAtMs,
    token: params.token ?? "",
    nonce: params.nonce ?? "",
  };
  return JSON.stringify(obj);
}
