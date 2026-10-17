import 'package:flutter/material.dart';

import 'theme.dart';

class App extends StatelessWidget {
  const App({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Job Manager',
      debugShowCheckedModeBanner: false,
      theme: AppTheme.light,
      darkTheme: AppTheme.dark,
      themeMode: ThemeMode.system,
      home: const LoginScreen(),
    );
  }
}

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key});

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _formKey = GlobalKey<FormState>();
  final _email = TextEditingController();
  final _password = TextEditingController();
  bool _obscurePassword = true;

  @override
  void dispose() {
    _email.dispose();
    _password.dispose();
    super.dispose();
  }

  void _login() {
    if (!_formKey.currentState!.validate()) return;
    Navigator.of(context).pushReplacement(
      MaterialPageRoute<void>(builder: (_) => const DashboardScreen()),
    );
  }

  @override
  Widget build(BuildContext context) => _AuthScaffold(
        child: Form(
          key: _formKey,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const _AuthHeader(
                eyebrow: 'WELCOME BACK',
                title: 'Sign in to Job Manager',
                subtitle: 'Manage your distributed jobs from anywhere.',
              ),
              const SizedBox(height: 30),
              TextFormField(
                controller: _email,
                keyboardType: TextInputType.emailAddress,
                decoration: const InputDecoration(
                  labelText: 'Email address',
                  prefixIcon: Icon(Icons.mail_outline_rounded),
                ),
                validator: (value) => _emailValidator(value),
              ),
              const SizedBox(height: 16),
              TextFormField(
                controller: _password,
                obscureText: _obscurePassword,
                decoration: InputDecoration(
                  labelText: 'Password',
                  prefixIcon: const Icon(Icons.lock_outline_rounded),
                  suffixIcon: IconButton(
                    onPressed: () => setState(() => _obscurePassword = !_obscurePassword),
                    icon: Icon(_obscurePassword ? Icons.visibility_outlined : Icons.visibility_off_outlined),
                  ),
                ),
                validator: (value) => value == null || value.length < 6 ? 'Use at least 6 characters.' : null,
              ),
              Align(
                alignment: Alignment.centerRight,
                child: TextButton(
                  onPressed: () => Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => const ForgotPasswordScreen())),
                  child: const Text('Forgot password?'),
                ),
              ),
              const SizedBox(height: 8),
              FilledButton(onPressed: _login, child: const Text('Sign in')),
              const SizedBox(height: 24),
              const _AuthDivider(),
              const SizedBox(height: 18),
              OutlinedButton(
                onPressed: () => Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => const CreateAccountScreen())),
                child: const Text('Create an account'),
              ),
            ],
          ),
        ),
      );
}

class ForgotPasswordScreen extends StatefulWidget {
  const ForgotPasswordScreen({super.key});

  @override
  State<ForgotPasswordScreen> createState() => _ForgotPasswordScreenState();
}

class _ForgotPasswordScreenState extends State<ForgotPasswordScreen> {
  final _formKey = GlobalKey<FormState>();
  final _email = TextEditingController();
  bool _sent = false;

  @override
  void dispose() {
    _email.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => _AuthScaffold(
        showBack: true,
        child: Form(
          key: _formKey,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const _AuthHeader(
                eyebrow: 'ACCOUNT RECOVERY',
                title: 'Forgot your password?',
                subtitle: 'Enter your email and we’ll send you a reset link.',
              ),
              const SizedBox(height: 30),
              if (_sent)
                const _MessageBox(text: 'Check your inbox for a password reset link.')
              else ...[
                TextFormField(
                  controller: _email,
                  keyboardType: TextInputType.emailAddress,
                  decoration: const InputDecoration(
                    labelText: 'Email address',
                    prefixIcon: Icon(Icons.mail_outline_rounded),
                  ),
                  validator: (value) => _emailValidator(value),
                ),
                const SizedBox(height: 20),
                FilledButton(
                  onPressed: () {
                    if (_formKey.currentState!.validate()) setState(() => _sent = true);
                  },
                  child: const Text('Send reset link'),
                ),
              ],
              const SizedBox(height: 24),
              TextButton(onPressed: () => Navigator.of(context).pop(), child: const Text('Back to sign in')),
            ],
          ),
        ),
      );
}

class CreateAccountScreen extends StatefulWidget {
  const CreateAccountScreen({super.key});

  @override
  State<CreateAccountScreen> createState() => _CreateAccountScreenState();
}

class _CreateAccountScreenState extends State<CreateAccountScreen> {
  final _formKey = GlobalKey<FormState>();
  final _name = TextEditingController();
  final _email = TextEditingController();
  final _password = TextEditingController();
  bool _obscurePassword = true;

  @override
  void dispose() {
    _name.dispose();
    _email.dispose();
    _password.dispose();
    super.dispose();
  }

  void _createAccount() {
    if (!_formKey.currentState!.validate()) return;
    Navigator.of(context).pushReplacement(MaterialPageRoute<void>(builder: (_) => const DashboardScreen()));
  }

  @override
  Widget build(BuildContext context) => _AuthScaffold(
        showBack: true,
        child: Form(
          key: _formKey,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const _AuthHeader(
                eyebrow: 'GET STARTED',
                title: 'Create your account',
                subtitle: 'Start tracking your distributed jobs in one place.',
              ),
              const SizedBox(height: 26),
              TextFormField(controller: _name, decoration: const InputDecoration(labelText: 'Full name', prefixIcon: Icon(Icons.person_outline_rounded)), validator: (value) => value == null || value.trim().isEmpty ? 'Enter your name.' : null),
              const SizedBox(height: 14),
              TextFormField(controller: _email, keyboardType: TextInputType.emailAddress, decoration: const InputDecoration(labelText: 'Email address', prefixIcon: Icon(Icons.mail_outline_rounded)), validator: (value) => _emailValidator(value)),
              const SizedBox(height: 14),
              TextFormField(controller: _password, obscureText: _obscurePassword, decoration: InputDecoration(labelText: 'Password', prefixIcon: const Icon(Icons.lock_outline_rounded), suffixIcon: IconButton(onPressed: () => setState(() => _obscurePassword = !_obscurePassword), icon: Icon(_obscurePassword ? Icons.visibility_outlined : Icons.visibility_off_outlined))), validator: (value) => value == null || value.length < 6 ? 'Use at least 6 characters.' : null),
              const SizedBox(height: 20),
              FilledButton(onPressed: _createAccount, child: const Text('Create account')),
              const SizedBox(height: 18),
              TextButton(onPressed: () => Navigator.of(context).pop(), child: const Text('Already have an account? Sign in')),
            ],
          ),
        ),
      );
}

String? _emailValidator(String? value) {
  if (value == null || value.trim().isEmpty) return 'Enter your email address.';
  if (!value.contains('@') || !value.contains('.')) return 'Enter a valid email address.';
  return null;
}

class _AuthScaffold extends StatelessWidget {
  const _AuthScaffold({required this.child, this.showBack = false});
  final Widget child;
  final bool showBack;

  @override
  Widget build(BuildContext context) => Scaffold(
        appBar: showBack ? AppBar(leading: BackButton(onPressed: () => Navigator.of(context).pop())) : null,
        body: SafeArea(
          child: Center(
            child: SingleChildScrollView(
              padding: const EdgeInsets.all(24),
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 440),
                child: Column(
                  children: [
                    if (!showBack) ...[
                      Container(width: 58, height: 58, decoration: BoxDecoration(color: Theme.of(context).colorScheme.primary, borderRadius: BorderRadius.circular(17)), child: const Icon(Icons.hub_rounded, color: Colors.white, size: 30)),
                      const SizedBox(height: 14),
                      Text('Job Manager', style: Theme.of(context).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800)),
                      const SizedBox(height: 28),
                    ],
                    Card(child: Padding(padding: const EdgeInsets.all(24), child: child)),
                  ],
                ),
              ),
            ),
          ),
        ),
      );
}

class _AuthHeader extends StatelessWidget {
  const _AuthHeader({required this.eyebrow, required this.title, required this.subtitle});
  final String eyebrow, title, subtitle;

  @override
  Widget build(BuildContext context) => Column(crossAxisAlignment: CrossAxisAlignment.start, children: [Text(eyebrow, style: Theme.of(context).textTheme.labelSmall?.copyWith(letterSpacing: 1.5, fontWeight: FontWeight.bold, color: Theme.of(context).colorScheme.primary)), const SizedBox(height: 7), Text(title, style: Theme.of(context).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w800)), const SizedBox(height: 8), Text(subtitle, style: Theme.of(context).textTheme.bodyMedium)]);
}

class _AuthDivider extends StatelessWidget {
  const _AuthDivider();

  @override
  Widget build(BuildContext context) => Row(children: [const Expanded(child: Divider()), Padding(padding: const EdgeInsets.symmetric(horizontal: 12), child: Text('OR', style: Theme.of(context).textTheme.labelSmall)), const Expanded(child: Divider())]);
}

class _MessageBox extends StatelessWidget {
  const _MessageBox({required this.text});
  final String text;

  @override
  Widget build(BuildContext context) => Container(padding: const EdgeInsets.all(14), decoration: BoxDecoration(color: Theme.of(context).colorScheme.primaryContainer, borderRadius: BorderRadius.circular(12)), child: Row(children: [Icon(Icons.check_circle_outline, color: Theme.of(context).colorScheme.primary), const SizedBox(width: 10), Expanded(child: Text(text))]));
}

class DashboardScreen extends StatefulWidget {
  const DashboardScreen({super.key});

  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen> {
  int _selectedTab = 0;
  String _selectedOperation = 'Addition';
  final _firstNumber = TextEditingController(text: '12');
  final _secondNumber = TextEditingController(text: '8');
  final _inputPath = TextEditingController(text: 'sample.csv');
  final _jobId = TextEditingController();
  final _jobs = <DemoJob>[
    DemoJob('job-1710429102', 'Addition', '12 + 8', 'completed', '20', '2 min ago'),
    DemoJob('job-1710429046', 'Prime count', 'up to 250000', 'processing', '—', '3 min ago'),
    DemoJob('job-1710428971', 'Retry demo', '2 retries → 42', 'queued', '—', '5 min ago'),
    DemoJob('job-1710428810', 'Multiplication', '24 × 16', 'failed', '—', '8 min ago'),
  ];

  @override
  void dispose() {
    _firstNumber.dispose();
    _secondNumber.dispose();
    _inputPath.dispose();
    _jobId.dispose();
    super.dispose();
  }

  void _submit() {
    final isFileJob = _selectedOperation == 'Image processing' || _selectedOperation == 'Log analysis' || _selectedOperation == 'CSV processing';
    final expression = isFileJob ? _inputPath.text : '${_firstNumber.text} $_operationSymbol ${_secondNumber.text}';
    setState(() => _jobs.insert(0, DemoJob(
      'job-${DateTime.now().millisecondsSinceEpoch}', _selectedOperation,
      expression, 'queued', '—', 'just now',
    )));
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Job added to the queue')));
  }

  String get _operationSymbol => switch (_selectedOperation) {
    'Multiplication' => '×', 'Subtraction' => '−', 'Integer division' => '÷', _ => '+',
  };

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        titleSpacing: 20,
        title: Row(children: [
          Container(width: 30, height: 30, decoration: BoxDecoration(color: Theme.of(context).colorScheme.primary, borderRadius: BorderRadius.circular(9)), child: const Icon(Icons.hub_rounded, color: Colors.white, size: 18)),
          const SizedBox(width: 10), const Text('Job Manager', style: TextStyle(fontWeight: FontWeight.w700)),
        ]),
        actions: [IconButton(onPressed: () {}, icon: const Icon(Icons.notifications_none_rounded)), const SizedBox(width: 4), const CircleAvatar(radius: 17, child: Text('M')), const SizedBox(width: 20)],
      ),
      body: Row(children: [
        _NavigationRail(selected: _selectedTab, onSelected: (index) => setState(() => _selectedTab = index)),
        Expanded(child: LayoutBuilder(builder: (context, constraints) => SingleChildScrollView(padding: EdgeInsets.symmetric(horizontal: constraints.maxWidth > 900 ? 36 : 20, vertical: 28), child: ConstrainedBox(constraints: const BoxConstraints(maxWidth: 1120), child: _selectedTab == 0 ? _buildDashboard(context, constraints.maxWidth) : _buildPlaceholder(context))))),
      ]),
    );
  }

  Widget _buildDashboard(BuildContext context, double width) {
    final compact = width < 720;
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Text('OPERATIONS / JOBS', style: Theme.of(context).textTheme.labelSmall?.copyWith(letterSpacing: 1.6, fontWeight: FontWeight.bold, color: Theme.of(context).colorScheme.primary)),
      const SizedBox(height: 7), Text('Job queue', style: Theme.of(context).textTheme.displaySmall?.copyWith(fontWeight: FontWeight.w800)),
      const SizedBox(height: 7), Text('Submit work and follow each attempt from the queue to completion.', style: Theme.of(context).textTheme.bodyLarge), const SizedBox(height: 28),
      _SummaryRow(jobs: _jobs, compact: compact), const SizedBox(height: 24),
      if (compact) ...[_SubmitCard(operation: _selectedOperation, onOperationChanged: (value) => setState(() => _selectedOperation = value ?? _selectedOperation), first: _firstNumber, second: _secondNumber, inputPath: _inputPath, onSubmit: _submit), const SizedBox(height: 20), _HistoryCard(jobs: _jobs, onRemove: (job) => setState(() => _jobs.remove(job)))]
      else Row(crossAxisAlignment: CrossAxisAlignment.start, children: [SizedBox(width: 330, child: _SubmitCard(operation: _selectedOperation, onOperationChanged: (value) => setState(() => _selectedOperation = value ?? _selectedOperation), first: _firstNumber, second: _secondNumber, inputPath: _inputPath, onSubmit: _submit)), const SizedBox(width: 20), Expanded(child: _HistoryCard(jobs: _jobs, onRemove: (job) => setState(() => _jobs.remove(job))) )]),
    ]);
  }

  Widget _buildPlaceholder(BuildContext context) => Center(child: Padding(padding: const EdgeInsets.only(top: 100), child: Column(children: [Icon(_selectedTab == 1 ? Icons.analytics_outlined : Icons.settings_outlined, size: 56, color: Theme.of(context).colorScheme.primary), const SizedBox(height: 18), Text(_selectedTab == 1 ? 'Analytics' : 'Settings', style: Theme.of(context).textTheme.headlineMedium), const SizedBox(height: 8), const Text('This section is coming next.')])));
}

class DemoJob {
  const DemoJob(this.id, this.operation, this.expression, this.status, this.result, this.age);
  final String id, operation, expression, status, result, age;
}

class _NavigationRail extends StatelessWidget {
  const _NavigationRail({required this.selected, required this.onSelected});
  final int selected; final ValueChanged<int> onSelected;
  @override
  Widget build(BuildContext context) => NavigationRail(selectedIndex: selected, onDestinationSelected: onSelected, labelType: NavigationRailLabelType.all, leading: const SizedBox(height: 12), destinations: const [NavigationRailDestination(icon: Icon(Icons.dashboard_outlined), selectedIcon: Icon(Icons.dashboard_rounded), label: Text('Jobs')), NavigationRailDestination(icon: Icon(Icons.bar_chart_outlined), selectedIcon: Icon(Icons.bar_chart_rounded), label: Text('Stats')), NavigationRailDestination(icon: Icon(Icons.settings_outlined), selectedIcon: Icon(Icons.settings_rounded), label: Text('Settings'))]);
}

class _SummaryRow extends StatelessWidget {
  const _SummaryRow({required this.jobs, required this.compact});
  final List<DemoJob> jobs; final bool compact;
  @override
  Widget build(BuildContext context) { final counts = {for (final status in ['queued', 'processing', 'completed', 'failed']) status: jobs.where((job) => job.status == status).length}; final cards = [('Tracked jobs', jobs.length, Icons.swap_horiz_rounded), ('Queued', counts['queued']!, Icons.more_horiz_rounded), ('Processing', counts['processing']!, Icons.timelapse_rounded), ('Completed', counts['completed']!, Icons.check_rounded), ('Failed', counts['failed']!, Icons.priority_high_rounded)]; return GridView.count(crossAxisCount: compact ? 2 : 5, shrinkWrap: true, physics: const NeverScrollableScrollPhysics(), crossAxisSpacing: 12, mainAxisSpacing: 12, childAspectRatio: compact ? 2.2 : 1.5, children: cards.map((card) => Card(child: Padding(padding: const EdgeInsets.all(16), child: Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [Text(card.$1, style: Theme.of(context).textTheme.labelMedium), Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [Text('${card.$2}', style: Theme.of(context).textTheme.headlineMedium?.copyWith(fontWeight: FontWeight.w800)), Icon(card.$3, color: Theme.of(context).colorScheme.primary)])])))).toList()); }
}

class _SubmitCard extends StatelessWidget {
  const _SubmitCard({required this.operation, required this.onOperationChanged, required this.first, required this.second, required this.inputPath, required this.onSubmit});
  final String operation; final ValueChanged<String?> onOperationChanged; final TextEditingController first, second, inputPath; final VoidCallback onSubmit;
  @override
  Widget build(BuildContext context) { final isFileJob = operation == 'Image processing' || operation == 'Log analysis' || operation == 'CSV processing'; return Card(child: Padding(padding: const EdgeInsets.all(20), child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [Text('NEW TASK', style: Theme.of(context).textTheme.labelSmall?.copyWith(letterSpacing: 1.5, fontWeight: FontWeight.bold)), const SizedBox(height: 6), Text('Submit a job', style: Theme.of(context).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700)), const SizedBox(height: 20), DropdownButtonFormField<String>(initialValue: operation, decoration: const InputDecoration(labelText: 'Operation'), items: ['Addition', 'Multiplication', 'Subtraction', 'Integer division', 'Fibonacci', 'Prime count', 'Retry demo', 'Image processing', 'Log analysis', 'CSV processing'].map((value) => DropdownMenuItem(value: value, child: Text(value))).toList(), onChanged: onOperationChanged), const SizedBox(height: 8), Text(isFileJob ? 'Use a path relative to the server JOB_DATA_DIR.' : 'Adds the two numbers and returns the result.', style: Theme.of(context).textTheme.bodySmall), const SizedBox(height: 16), if (isFileJob) TextField(controller: inputPath, decoration: const InputDecoration(labelText: 'Local input path', prefixIcon: Icon(Icons.insert_drive_file_outlined))) else Row(children: [Expanded(child: TextField(controller: first, keyboardType: TextInputType.number, decoration: const InputDecoration(labelText: 'First number'))), const Padding(padding: EdgeInsets.all(10), child: Text('+')), Expanded(child: TextField(controller: second, keyboardType: TextInputType.number, decoration: const InputDecoration(labelText: 'Second number')))]), const SizedBox(height: 18), FilledButton.icon(onPressed: onSubmit, icon: const Icon(Icons.arrow_forward_rounded), label: const Text('Submit job')), const SizedBox(height: 20), const Divider(), const SizedBox(height: 12), Text('History is stored on this device.', style: Theme.of(context).textTheme.bodySmall)]))); }
}

class _HistoryCard extends StatelessWidget {
  const _HistoryCard({required this.jobs, required this.onRemove});
  final List<DemoJob> jobs; final ValueChanged<DemoJob> onRemove;
  @override
  Widget build(BuildContext context) => Card(child: Padding(padding: const EdgeInsets.fromLTRB(20, 20, 12, 12), child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [Text('Job history', style: Theme.of(context).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700)), TextButton.icon(onPressed: () {}, icon: const Icon(Icons.refresh, size: 18), label: const Text('Refresh'))]), const SizedBox(height: 8), if (jobs.isEmpty) const Padding(padding: EdgeInsets.all(40), child: Center(child: Text('Your queue is clear.'))) else ...jobs.map((job) => _JobRow(job: job, onRemove: () => onRemove(job)))])));
}

class _JobRow extends StatelessWidget {
  const _JobRow({required this.job, required this.onRemove}); final DemoJob job; final VoidCallback onRemove;
  @override
  Widget build(BuildContext context) { final color = switch (job.status) { 'completed' => Colors.green, 'failed' => Colors.red, 'processing' => Colors.orange, _ => Theme.of(context).colorScheme.primary }; return ListTile(contentPadding: EdgeInsets.zero, leading: CircleAvatar(backgroundColor: color.withAlpha(25), foregroundColor: color, child: Icon(job.status == 'completed' ? Icons.check : Icons.more_horiz)), title: Text(job.operation, style: const TextStyle(fontWeight: FontWeight.w600)), subtitle: Text('${job.id}\n${job.expression}  ·  ${job.age}'), isThreeLine: true, trailing: Wrap(crossAxisAlignment: WrapCrossAlignment.center, spacing: 4, children: [if (job.result != '—') Text(job.result, style: Theme.of(context).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold)), Chip(label: Text(job.status), visualDensity: VisualDensity.compact), IconButton(onPressed: onRemove, icon: const Icon(Icons.close, size: 18))])); }
}
