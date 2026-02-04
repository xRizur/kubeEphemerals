/**
 * Creating a sidebar enables you to:
 - create an ordered group of docs
 - render a sidebar for each doc of that group
 - provide next/previous navigation
 */

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  tutorialSidebar: [
    'intro',
    'installation',
    {
      type: 'category',
      label: 'CRD Reference',
      items: [
        'crd-ephemeralenv',
        'crd-environmenttemplate',
      ],
    },
    'network-policy',
    'gateway-httproute',
    'helm-values',
    'kubeconfig',
    'securing-previews',
  ],
};

export default sidebars;
