import { fireEvent, render, screen } from '@testing-library/react-native';

import { AmountInput } from '@/ui/AmountInput';
import { colors } from '@/ui/theme';

describe('AmountInput', () => {
  it('shows the label and names the field by it, on a number pad', async () => {
    await render(<AmountInput label="Amount" value="" onChangeText={jest.fn()} />);
    expect(screen.getByText('Amount')).toBeOnTheScreen();
    expect(screen.getByLabelText('Amount')).toHaveProp('keyboardType', 'number-pad');
  });

  it('shows the value it is given and hands typing to the caller', async () => {
    const onChangeText = jest.fn();
    await render(<AmountInput label="Amount" value="100.000" onChangeText={onChangeText} />);
    const input = screen.getByLabelText('Amount');
    expect(input).toHaveDisplayValue('100.000');
    // fireEvent falls back to handlers on composite parents, so check the field itself is wired.
    expect(input).toHaveProp('onChangeText', onChangeText);
    await fireEvent.changeText(input, '1000005');
    expect(onChangeText).toHaveBeenCalledWith('1000005');
  });

  it('marks the border while focused and clears it on blur', async () => {
    await render(<AmountInput label="Amount" value="" onChangeText={jest.fn()} />);
    const input = screen.getByLabelText('Amount');
    expect(input).toHaveStyle({ borderColor: colors.surface });
    await fireEvent(input, 'focus');
    expect(input).toHaveStyle({ borderColor: colors.primary });
    await fireEvent(input, 'blur');
    expect(input).toHaveStyle({ borderColor: colors.surface });
  });
});
