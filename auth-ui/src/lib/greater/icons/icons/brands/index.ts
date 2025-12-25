/**
 * Brand Icons - OAuth Providers and Social Media
 *
 * IMPORTANT: These are brand icons and should be used according to each
 * company's brand guidelines. See individual icon files for specific
 * licensing and usage information.
 *
 * Brand Guidelines:
 * - Google: https://developers.google.com/identity/branding-guidelines
 * - Apple: https://developer.apple.com/design/human-interface-guidelines/sign-in-with-apple
 * - Microsoft: https://docs.microsoft.com/en-us/azure/active-directory/develop/howto-add-branding-in-azure-ad-apps
 * - Discord: https://discord.com/branding
 * - GitHub: https://github.com/logos
 * - GitLab: https://about.gitlab.com/press/press-kit/
 * - Slack: https://slack.com/media-kit
 * - LinkedIn: https://brand.linkedin.com/
 */

// OAuth Provider Icons (new fill-based icons)
export { default as GoogleIcon } from './google.svelte';
export { default as AppleIcon } from './apple.svelte';
export { default as MicrosoftIcon } from './microsoft.svelte';
export { default as DiscordIcon } from './discord.svelte';

// Re-export existing social icons from main icons directory
export { default as GithubIcon } from '../github.svelte';
export { default as GitlabIcon } from '../gitlab.svelte';
export { default as SlackIcon } from '../slack.svelte';
export { default as LinkedinIcon } from '../linkedin.svelte';
export { default as TwitterIcon } from '../twitter.svelte';
export { default as FacebookIcon } from '../facebook.svelte';
export { default as InstagramIcon } from '../instagram.svelte';
export { default as YoutubeIcon } from '../youtube.svelte';
export { default as TwitchIcon } from '../twitch.svelte';

export const brandIconList = [
	'google',
	'apple',
	'microsoft',
	'discord',
	'github',
	'gitlab',
	'slack',
	'linkedin',
	'twitter',
	'facebook',
	'instagram',
	'youtube',
	'twitch',
] as const;

export type BrandIconName = (typeof brandIconList)[number];
