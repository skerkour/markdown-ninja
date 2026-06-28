let config: Config | null = null;

export function createConfig(): Config {
  config = new Config();
  return config;
}

export function useConfig(): Config {
  if (!config) {
    throw new Error('Config should be created before using it');
  }
  return config!;
}

/* eslint-disable @typescript-eslint/no-non-null-assertion */
export class Config {
  env: string;
  sitesPort: string;
  oidcRedirectUri: string;
  cmsBaseUrl: string;
  githubRepository: string;
  docsUrl: string;

  constructor() {
    this.env = import.meta.env.VITE_ENV as string | undefined ?? 'production';
    this.sitesPort = this.env === 'dev' ? ':4000' : '';
    // get the OIDC redirect URI as [protocol]//[host]/auth (e.g [https:]//[markdown.ninja]/auth)
    // This is secure as redirect_uris need to be allowlisted on the OIDC server
    this.oidcRedirectUri = `${window.location.protocol}//${window.location.host}/auth`;
    this.cmsBaseUrl = "https://cms.markdown.ninja";
    this.githubRepository = "https://github.com/skerkour/markdown-ninja";
    this.docsUrl = "https://docs.markdown.ninja";
  }
}
