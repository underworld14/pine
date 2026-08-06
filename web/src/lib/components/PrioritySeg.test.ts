import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import PrioritySeg from './PrioritySeg.svelte';

describe('PrioritySeg', () => {
  it('renders short labels for every priority', () => {
    render(PrioritySeg, { props: { value: 'medium', onChange: vi.fn() } });
    expect(screen.getByTestId('prio-low').textContent).toContain('Low');
    expect(screen.getByTestId('prio-medium').textContent).toContain('Med');
    expect(screen.getByTestId('prio-high').textContent).toContain('High');
    expect(screen.getByTestId('prio-critical').textContent).toContain('Crit');
  });

  it('marks the current value pressed and calls onChange', async () => {
    const onChange = vi.fn();
    render(PrioritySeg, { props: { value: 'low', onChange, testIdPrefix: 'peek-prio' } });
    expect(screen.getByTestId('peek-prio-low').getAttribute('aria-pressed')).toBe('true');
    expect(screen.getByTestId('peek-prio-high').getAttribute('aria-pressed')).toBe('false');
    await fireEvent.click(screen.getByTestId('peek-prio-high'));
    expect(onChange).toHaveBeenCalledWith('high');
  });

  it('exposes full priority names via aria-label (no tooltip dependency)', () => {
    render(PrioritySeg, { props: { value: 'critical', onChange: vi.fn() } });
    expect(screen.getByLabelText('Critical')).toBeTruthy();
    expect(screen.getByLabelText('Medium')).toBeTruthy();
  });
});
