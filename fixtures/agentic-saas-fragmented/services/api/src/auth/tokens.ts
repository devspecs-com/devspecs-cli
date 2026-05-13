type TokenRefreshResult = {
  sessionToken: string;
  rotated: boolean;
};

export async function refreshSessionToken(userId: string): Promise<TokenRefreshResult> {
  return {
    sessionToken: `session:${userId}:rotated`,
    rotated: true,
  };
}

export function isSessionToken(value: string) {
  return value.startsWith("session:");
}

