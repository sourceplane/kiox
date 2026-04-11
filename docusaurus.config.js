import { createRequire } from 'module';

const require = createRequire(import.meta.url);

const config = {
  title: 'tinx',
  tagline: 'OCI-native provider runtime, workspace shell, and packager',
  url: 'https://your-cloudflare-project.pages.dev',
  baseUrl: '/',
  organizationName: 'sourceplane',
  projectName: 'tinx',
  onBrokenLinks: 'throw',
  onDuplicateRoutes: 'throw',
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'throw',
    },
  },
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },
  presets: [
    [
      'classic',
      {
        docs: {
          path: 'docs',
          routeBasePath: '/',
          sidebarPath: require.resolve('./sidebars.js'),
        },
        blog: false,
        theme: {
          customCss: require.resolve('./src/css/custom.css'),
        },
      },
    ],
  ],
  themeConfig: {
    navbar: {
      title: 'tinx',
      items: [
        {
          to: '/',
          label: 'Documentation',
          position: 'left',
        },
        {
          href: 'https://github.com/sourceplane/tinx',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Getting Started',
          items: [
            { label: 'Installation', to: '/getting-started/installation' },
            { label: 'Quick Start', to: '/getting-started/quick-start' },
          ],
        },
        {
          title: 'Core Concepts',
          items: [
            { label: 'Workspace', to: '/concepts/workspace' },
            { label: 'Providers', to: '/concepts/providers' },
            { label: 'Runtime Shell', to: '/concepts/runtime-shell' },
          ],
        },
        {
          title: 'Reference',
          items: [
            { label: 'CLI', to: '/cli/tinx' },
            { label: 'Architecture', to: '/architecture/internals' },
            { label: 'Contributing', to: '/contributing/' },
          ],
        },
      ],
      copyright: `Copyright ${new Date().getFullYear()} sourceplane`,
    },
    prism: {
      additionalLanguages: ['bash', 'json', 'yaml'],
    },
  },
};

export default config;
