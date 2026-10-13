import 'package:flutter/material.dart';
import 'theme.dart';

class HubBrand extends StatelessWidget {
  final String subtitle;
  const HubBrand({super.key, required this.subtitle});

  @override
  Widget build(BuildContext context) => Row(children: [
        Container(
          width: 34,
          height: 34,
          decoration: BoxDecoration(
            color: HubTheme.accent,
            borderRadius: BorderRadius.circular(10),
          ),
          child:
              const Icon(Icons.insights_rounded, color: Colors.white, size: 22),
        ),
        const SizedBox(width: 10),
        Flexible(
          child:
              Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            const Text('AI Hub',
                style: TextStyle(
                    fontSize: 19,
                    fontWeight: FontWeight.w700,
                    letterSpacing: -0.5)),
            Text(subtitle,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(fontSize: 10, color: HubTheme.muted)),
          ]),
        ),
      ]);
}

class HubNavigationBar extends StatelessWidget {
  final int selectedIndex;
  final ValueChanged<int> onSelected;
  const HubNavigationBar(
      {super.key, required this.selectedIndex, required this.onSelected});

  @override
  Widget build(BuildContext context) => DecoratedBox(
        decoration: const BoxDecoration(
            border: Border(top: BorderSide(color: HubTheme.line))),
        child: NavigationBar(
          selectedIndex: selectedIndex,
          onDestinationSelected: onSelected,
          destinations: const [
            NavigationDestination(
                icon: Icon(Icons.grid_view_outlined),
                selectedIcon: Icon(Icons.grid_view_rounded),
                label: '总览'),
            NavigationDestination(
                icon: Icon(Icons.notifications_outlined),
                selectedIcon: Icon(Icons.notifications_rounded),
                label: '通知'),
            NavigationDestination(
                icon: Icon(Icons.edit_note_outlined),
                selectedIcon: Icon(Icons.edit_note_rounded),
                label: '便笺'),
            NavigationDestination(
                icon: Icon(Icons.devices_outlined),
                selectedIcon: Icon(Icons.devices_rounded),
                label: '设备'),
            NavigationDestination(
                icon: Icon(Icons.settings_outlined),
                selectedIcon: Icon(Icons.settings_rounded),
                label: '设置'),
          ],
        ),
      );
}
