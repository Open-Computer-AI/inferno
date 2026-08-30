/**
 * A proxy row must show the thing you searched for.
 *
 * The June rewrite reduced each dropdown row to host + "protocol · region" and
 * dropped proxy.name and account_count. But the search predicate still matches
 * on name -- so you typed a proxy's name, got the right rows back, and none of
 * them showed a name. The count of accounts using a proxy went with it.
 */
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ProxySelector from '../ProxySelector.vue'

const i18n = createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })

const proxies = [
  { id: 1, name: 'Frankfurt egress', host: 'fra-01.proxy.internal', protocol: 'socks5', region: 'eu-central', account_count: 4 },
  { id: 2, name: 'Singapore egress', host: 'sin-02.proxy.internal', protocol: 'http', region: 'ap-southeast', account_count: 0 }
] as never[]

const open = async (over: Record<string, unknown> = {}) => {
  const w = mount(ProxySelector, {
    props: { modelValue: null, proxies, ...over },
    global: { plugins: [i18n], stubs: { Icon: true, teleport: true } }
  })
  await w.find('button').trigger('click')
  return w
}

describe('ProxySelector row', () => {
  it('shows the name, which is what the search matches on', async () => {
    const w = await open()
    const names = w.findAll('.pxsel-row__name').map((n) => n.text())
    expect(names).toContain('Frankfurt egress')
    expect(names).toContain('Singapore egress')
  })

  it('still shows the host and its protocol/region line', async () => {
    const w = await open()
    expect(w.find('.pxsel-row__host').text()).toContain('fra-01.proxy.internal')
    expect(w.find('.pxsel-row__submeta').text()).toContain('socks5')
  })

  it('shows how many accounts use each proxy', async () => {
    const w = await open()
    expect(w.findAll('.pxsel-row__count').map((n) => n.text())).toEqual(['4', '0'])
  })

  it('renders a name for every row the search keeps', async () => {
    // The regression was exactly this pair being inconsistent: filter by name,
    // render without it.
    const w = await open()
    await w.find('input').setValue('Frankfurt')
    const rows = w.findAll('.pxsel-row__name')
    expect(rows).toHaveLength(1)
    expect(rows[0].text()).toBe('Frankfurt egress')
  })

  it('omits the count when the API did not send one', async () => {
    const w = await open({ proxies: [{ id: 3, name: 'No count', host: 'h', protocol: 'http' }] as never[] })
    expect(w.find('.pxsel-row__count').exists()).toBe(false)
  })
})
