using System;
using System.Globalization;
using System.Windows;
using System.Windows.Data;

namespace Aevora;

// Visibility = Collapsed when the bound bool is true (the inverse of the built-in
// BooleanToVisibilityConverter). Used for "show when NOT enrolled / NOT available".
public sealed class InverseBooleanToVisibilityConverter : IValueConverter
{
    public object Convert(object value, Type targetType, object parameter, CultureInfo culture)
        => (value is bool b && b) ? Visibility.Collapsed : Visibility.Visible;

    public object ConvertBack(object value, Type targetType, object parameter, CultureInfo culture)
        => value is Visibility v && v != Visibility.Visible;
}
