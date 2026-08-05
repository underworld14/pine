import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import BoardFilterBar from './BoardFilterBar.svelte';
import { emptyFilter } from '$lib/board-filter';

describe('BoardFilterBar', () => {
  it('emits text changes', async () => {
    const onChange = vi.fn();
    render(BoardFilterBar, { filter: emptyFilter(), labels: [], onChange });
    await fireEvent.input(screen.getByTestId('board-filter-text'), { target: { value: 'login' } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ text: 'login' }));
  });

  it('toggles a priority', async () => {
    const onChange = vi.fn();
    render(BoardFilterBar, { filter: emptyFilter(), labels: [], onChange });
    await fireEvent.click(screen.getByTestId('board-filter-prio-high'));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ priorities: ['high'] }));
  });

  it('renders label chips and toggles them', async () => {
    const onChange = vi.fn();
    render(BoardFilterBar, { filter: emptyFilter(), labels: ['ui', 'auth'], onChange });
    await fireEvent.click(screen.getByTestId('board-filter-label-auth'));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ labels: ['auth'] }));
  });

  it('shows a clear button only when the filter is active', () => {
    const { rerender } = render(BoardFilterBar, { filter: emptyFilter(), labels: [], onChange: vi.fn() });
    expect(screen.queryByTestId('board-filter-clear')).toBeNull();
    rerender({ filter: { text: 'x', labels: [], priorities: [] }, labels: [], onChange: vi.fn() });
    expect(screen.getByTestId('board-filter-clear')).toBeTruthy();
  });
});
