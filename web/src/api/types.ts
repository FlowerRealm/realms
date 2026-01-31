export type APIResponse<T> = {
  success: boolean;
  message?: string;
  data?: T;
};

export type FeatureFlags = {
  web_announcements_disabled?: boolean;
  web_tokens_disabled?: boolean;
  web_usage_disabled?: boolean;
  models_disabled?: boolean;
  billing_disabled?: boolean;
  tickets_disabled?: boolean;

  admin_channels_disabled?: boolean;
  admin_channel_groups_disabled?: boolean;
  admin_users_disabled?: boolean;
  admin_usage_disabled?: boolean;
  admin_announcements_disabled?: boolean;
};

export type User = {
  id: number;
  email?: string;
  username?: string;
  role?: string;
  status?: number;
  groups?: string[];

  self_mode?: boolean;
  email_verification_enabled?: boolean;
  features?: FeatureFlags;
};
