import { describe, expect, it } from 'vite-plus/test'
import { mount } from '@vue/test-utils'
import DropdownSelect from '../src/DropdownSelect.vue'

function dropdown() {
  return mount(DropdownSelect, {
    props: {
      id: 'role',
      label: 'Role',
      modelValue: 'viewer',
      options: [
        { value: 'viewer', label: 'Viewer' },
        { value: 'editor', label: 'Editor' },
        { value: 'admin', label: 'Administrator' },
      ],
    },
  })
}

describe('dropdown selection', () => {
  it('selects with the keyboard without submitting the form', async () => {
    const wrapper = dropdown()
    const trigger = wrapper.get('[role="combobox"]')
    expect(trigger.attributes('type')).toBe('button')
    expect(trigger.text()).toBe('Viewer')
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    expect(trigger.attributes('aria-expanded')).toBe('true')
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await trigger.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([['editor']])
    expect(trigger.attributes('aria-expanded')).toBe('false')
    wrapper.unmount()
  })

  it('supports boundaries, typeahead, Escape and Tab', async () => {
    const wrapper = dropdown()
    const trigger = wrapper.get('[role="combobox"]')
    await trigger.trigger('keydown', { key: 'End' })
    expect(wrapper.get('[data-active="true"]').text()).toBe('Administrator')
    await trigger.trigger('keydown', { key: 'Home' })
    expect(wrapper.get('[data-active="true"]').text()).toBe('Viewer')
    await trigger.trigger('keydown', { key: 'e' })
    expect(wrapper.get('[data-active="true"]').text()).toBe('Editor')
    await trigger.trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    await trigger.trigger('click')
    await trigger.trigger('keydown', { key: 'Tab' })
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('selects by pointer, closes on blur and disables while saving', async () => {
    const wrapper = dropdown()
    const trigger = wrapper.get('[role="combobox"]')
    await trigger.trigger('click')
    expect(wrapper.get('[aria-selected="true"]').text()).toBe('Viewer')
    await wrapper.findAll('[role="option"]')[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([['editor']])
    await trigger.trigger('click')
    await trigger.trigger('blur')
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    await trigger.trigger('click')
    await wrapper.setProps({ disabled: true })
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    expect(trigger.attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
})
