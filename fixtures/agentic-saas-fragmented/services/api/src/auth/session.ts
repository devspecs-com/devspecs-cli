type Session = {
  userId: string;
  customer_id?: string;
  authorization_details?: unknown;
};

export function requireSession(session: Session | null) {
  if (!session) {
    throw new Error("unauthorized");
  }
  return session;
}

