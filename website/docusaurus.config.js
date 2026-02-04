// @ts-check
// `@type` JSDoc annotations allow editor autocompletion and type checking
// (when a type package is installed for the theme).

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Ephemeral Operator',
  tagline: 'Instant, Isolated Kubernetes Environments for every PR.',
  favicon: 'img/favicon.ico',

  url: 'https://maciekmm.github.io',
  baseUrl: '/kubeEphemerals/',

  organizationName: 'maciekmm',
  projectName: 'kubeEphemerals',

  onBrokenLinks: 'throw',
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          routeBasePath: 'docs',
          sidebarPath: './sidebars.js',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      colorMode: {
        defaultMode: 'light',
        respectPrefersColorScheme: true,
      },
      navbar: {
        title: 'Ephemeral Operator',
        logo: {
          alt: 'Ephemeral Operator',
          src: 'img/logo.png',
        },
        items: [
          { to: '/docs/intro', label: 'Docs', position: 'left' },
          {
            href: 'https://github.com/maciekmm/kubeEphemerals',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Docs',
            items: [
              { label: 'Introduction', to: '/docs/intro' },
              { label: 'Installation', to: '/docs/installation' },
            ],
          },
          {
            title: 'More',
            items: [
              { label: 'GitHub', href: 'https://github.com/maciekmm/kubeEphemerals' },
            ],
          },
        ],
        copyright: `Copyright © ${new Date().getFullYear()} Ephemeral Operator. Built with Docusaurus.`,
      },
    }),
};

export default config;
